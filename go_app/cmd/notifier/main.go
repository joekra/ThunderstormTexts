package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"github.com/twilio/twilio-go"
	openapi "github.com/twilio/twilio-go/rest/api/v2010"

	"thunderstorm-alerts/internal/alert"
	"thunderstorm-alerts/internal/config"
	"thunderstorm-alerts/internal/redis"
)

// Constants
const (
	NotificationCooldown = 2 * time.Hour
	RedisLastTimeKey     = "last_notification_time"
)

// Notifier handles sending SMS notifications
type Notifier struct {
	cfg         *config.Config
	redisClient *redis.Client
	twilio      *twilio.RestClient
}

// NewNotifier creates a new Notifier instance
func NewNotifier(cfg *config.Config, rClient *redis.Client) *Notifier {
	return &Notifier{
		cfg:         cfg,
		redisClient: rClient,
		twilio: twilio.NewRestClientWithParams(twilio.ClientParams{
			Username: cfg.TwilioAccountSID,
			Password: cfg.TwilioAuthToken,
		}),
	}
}

// getLastNotificationTime retrieves the timestamp of the last sent notification
func (n *Notifier) getLastNotificationTime(ctx context.Context) time.Time {
	val, err := n.redisClient.Get(ctx, RedisLastTimeKey)
	if err != nil {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, val)
	if err != nil {
		return time.Time{}
	}
	return t
}

// setLastNotificationTime sets the current time as the last notification time
func (n *Notifier) setLastNotificationTime(ctx context.Context) {
	n.redisClient.Set(ctx, RedisLastTimeKey, time.Now().Format(time.RFC3339), 0)
}

// getHailSize extracts hail size as float64 from the warning
func (n *Notifier) getHailSize(warning alert.Warning) float64 {
	var hailSize float64
	if warning.HailSize != nil {
		switch v := warning.HailSize.(type) {
		case float64:
			hailSize = v
		case string:
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				hailSize = f
			}
		}
	}
	return hailSize
}

// shouldSendNotification determines if a notification should be sent based on cooldown and severity
func (n *Notifier) shouldSendNotification(ctx context.Context, warning alert.Warning) bool {
	lastTime := n.getLastNotificationTime(ctx)
	hailSize := n.getHailSize(warning)

	// Bypass cooldown if hail is very large (>= 1.75")
	bypassCooldown := hailSize >= 1.75
	if bypassCooldown {
		log.Println("Bypassing cooldown due to hail size >= 1.75\"")
		return true
	}

	// If no previous notification and status is Alert, send it
	// Send first alert of the day, or any first update if first notification
	if lastTime.IsZero() && (warning.Status == "Alert" || warning.Status == "Update") {
	    return true
	}

	// If it's an Update, only send if hail size is significant (>= 1.75),
	// but we already returned true above if hailSize >= 1.75.

	// Check if cooldown period has passed
	return time.Since(lastTime) >= NotificationCooldown
}

// formatMessage formats the warning data into a readable SMS message
func (n *Notifier) formatMessage(w alert.Warning) string {
	areas := ""
	for i, area := range w.Areas {
		if i > 0 {
			areas += ", "
		}
		areas += area
	}

	hailMsg := ""
	hailSize := n.getHailSize(w)

	if hailSize > 0 {
		if hailSize >= 1.75 {
			if hailSize >= 2.5 {
				hailMsg = fmt.Sprintf("⚠️ Very Large Hail: %.2f\" ⚠️", hailSize)
			} else {
				hailMsg = fmt.Sprintf("⚠️ Large Hail: %.2f\" ⚠️", hailSize)
			}
		} else {
			hailMsg = fmt.Sprintf("Hail: %.2f\"", hailSize)
		}
	}

	// Construct message
	// If hailMsg is empty, we get an empty line which matches Python behavior.
	msg := fmt.Sprintf("⚠️ Severe Thunderstorm ⚠️\nStatus: %s\n%s\nFor: %s\nIssued: %s\nFrom: %s",
		w.Status, hailMsg, areas, w.Time, w.Office)

	return msg
}

// sendSMS sends a single SMS message
func (n *Notifier) sendSMS(to, body string) error {
	params := &openapi.CreateMessageParams{}
	params.SetTo(to)
	params.SetFrom(n.cfg.TwilioFromNumber)
	params.SetBody(body)

	_, err := n.twilio.Api.CreateMessage(params)
	return err
}

// sendNotifications sends SMS to all contacts
func (n *Notifier) sendNotifications(ctx context.Context, warning alert.Warning) {
	if n.cfg.TwilioAccountSID == "" || n.cfg.TwilioAuthToken == "" || n.cfg.TwilioFromNumber == "" {
		log.Println("Error: Twilio credentials not configured")
		return
	}

	if !n.shouldSendNotification(ctx, warning) {
		log.Println("Skipping notification: cooldown period not elapsed")
		return
	}

	msg := n.formatMessage(warning)
	log.Printf("Sending message content:\n%s", msg)

	for name, number := range n.cfg.Contacts {
		if err := n.sendSMS(number, msg); err != nil {
			log.Printf("Error sending SMS to %s (%s): %v", name, number, err)
		} else {
			log.Printf("Successfully sent SMS to %s (%s)", name, number)
		}
	}

	n.setLastNotificationTime(ctx)
}

// sendTestMessage sends a startup test message if not already sent today
func (n *Notifier) sendTestMessage(ctx context.Context) {
	if n.cfg.TwilioAccountSID == "" || n.cfg.TwilioAuthToken == "" || n.cfg.TwilioFromNumber == "" {
		log.Println("Error: Twilio credentials not configured")
		return
	}

	todayKey := fmt.Sprintf("test_message_sent:%s", time.Now().Format("2006-01-02"))
	_, err := n.redisClient.Get(ctx, todayKey)
	if err == nil {
		// Key exists: we already sent today
		log.Println("Test message already sent today, skipping...")
		return
	}
	if err != goredis.Nil {
		// Redis error (e.g. connection refused at startup): skip sending to avoid duplicate test messages
		log.Printf("Redis unavailable for test-message check (%v), skipping test message to avoid duplicate sends", err)
		return
	}
	// err == goredis.Nil: key missing, ok to send

	msg := "⚠️ This is a test message from the Hail Alert System. ⚠️"
	for name, number := range n.cfg.Contacts {
		if err := n.sendSMS(number, msg); err != nil {
			log.Printf("Error sending test SMS to %s (%s): %v", name, number, err)
		} else {
			log.Printf("Test message sent to %s (%s)", name, number)
		}
	}

	// Expire after 24 hours
	n.redisClient.Set(ctx, todayKey, "1", 24*time.Hour)
}

func main() {
	// Load configuration
	cfg := config.Load()

	// Initialize Redis connection
	rClient := redis.NewClient(cfg.RedisHost, cfg.RedisPort, cfg.RedisDB)
	defer rClient.Close()

	ctx := context.Background()

	log.Println("Connecting to Redis...")
	if err := rClient.WaitReady(ctx, redis.DefaultWaitReadyMaxWait, redis.DefaultWaitReadyBackoff, log.Printf); err != nil {
		log.Fatalf("Redis unavailable after %v: %v", redis.DefaultWaitReadyMaxWait, err)
	}
	log.Println("Redis connected.")

	// Initialize Notifier
	notifier := NewNotifier(cfg, rClient)

	log.Println("Starting SMS notifier...")

	// Send test message on startup
	log.Println("Sending test SMS to all contacts...")
	notifier.sendTestMessage(ctx)

	log.Println("Listening for new weather alerts in real-time...")

	// Subscribe to Redis channel
	pubsub := rClient.Subscribe(ctx, "weather_alerts")
	defer pubsub.Close()

	// Consume messages
	ch := pubsub.Channel()
	for msg := range ch {
		var warning alert.Warning
		if err := json.Unmarshal([]byte(msg.Payload), &warning); err != nil {
			log.Printf("Error parsing warning message: %v", err)
			continue
		}

		log.Println("Received new severe thunderstorm warning")
		notifier.sendNotifications(ctx, warning)
	}
}
