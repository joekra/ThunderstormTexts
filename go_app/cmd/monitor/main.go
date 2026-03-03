package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"thunderstorm-alerts/internal/alert"
	"thunderstorm-alerts/internal/config"
	"thunderstorm-alerts/internal/redis"
)

// Constants
const (
	NWSBaseURL    = "https://api.weather.gov"
	UserAgent     = "WeatherAlertMonitor/1.0 (your-email@example.com)"
	CheckInterval = 30 * time.Second
)

type WeatherAlertMonitor struct {
	client        *http.Client
	redisClient   *redis.Client
	seenIDs       map[string]struct{}
	lastCheckTime time.Time
}

func NewWeatherAlertMonitor(redisClient *redis.Client) *WeatherAlertMonitor {
	return &WeatherAlertMonitor{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		redisClient:   redisClient,
		seenIDs:       make(map[string]struct{}),
		lastCheckTime: time.Now().UTC(),
	}
}

func (m *WeatherAlertMonitor) formatWarning(props map[string]any) alert.Warning {
	// Parse areas
	var areas []string

	if areaDesc, ok := props["areaDesc"].(string); ok {
		for s := range strings.SplitSeq(areaDesc, ",") {
			areas = append(areas, strings.TrimSpace(s))
		}
	}

	// Parse hail size
	var hailSize any
	if params, ok := props["parameters"].(map[string]any); ok {
		if maxHail, ok := params["maxHailSize"].([]any); ok && len(maxHail) > 0 {
			hailSize = maxHail[0]
		}
	}

	// Helper to safely get string
	getString := func(key, def string) string {
		if v, ok := props[key].(string); ok {
			return v
		}
		return def
	}

	// Parse sent time for display
	alertTime := time.Now().UTC()
	if sentStr, ok := props["sent"].(string); ok {
		if t, err := time.Parse(time.RFC3339, sentStr); err == nil {
			alertTime = t.UTC()
		}
	}

	return alert.Warning{
		Type:         "warning",
		Event:        "Severe Thunderstorm Warning",
		Areas:        areas,
		Description:  getString("description", ""),
		Instructions: getString("instruction", "TAKE COVER NOW! Move to an interior room on the lowest floor of a sturdy building."),
		Office:       getString("senderName", "NWS"),
		Time:         alertTime.Format("2006-01-02 15:04") + " UTC",
		HailSize:     hailSize,
		Status:       getString("messageType", ""),
	}
}

func (m *WeatherAlertMonitor) getAlerts(area string, forceFirstSend bool) ([]alert.Warning, error) {
	currentTime := time.Now().UTC()
	url := fmt.Sprintf("%s/alerts/active", NWSBaseURL)
	if area != "" {
		url += fmt.Sprintf("/area/%s", area)
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "application/geo+json")

	resp, err := m.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var data map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	var warnings []alert.Warning
	features, ok := data["features"].([]any)
	if !ok {
		return warnings, nil
	}

	for _, f := range features {
		feature, ok := f.(map[string]any)
		if !ok {
			continue
		}
		properties, ok := feature["properties"].(map[string]any)
		if !ok {
			continue
		}

		event, _ := properties["event"].(string)
		id, _ := feature["id"].(string)

		if strings.Contains(event, "Severe Thunderstorm Warning") {
			sentStr, _ := properties["sent"].(string)
			sentTime, err := time.Parse(time.RFC3339, sentStr)
			if err != nil {
				sentTime = time.Now()
			}

			// Check condition: force send OR (sent recently AND not seen)
			threshold := m.lastCheckTime.Add(-30 * time.Second)

			if _, seen := m.seenIDs[id]; !seen {
				if forceFirstSend || sentTime.After(threshold) {
					m.seenIDs[id] = struct{}{}

					warning := m.formatWarning(properties)
					warnings = append(warnings, warning)

					msgType := warning.Status
					if msgType == "Alert" || msgType == "Update" {
						// Publish to Redis
						ctx := context.Background()
						jsonBytes, _ := json.Marshal(warning)

						if err := m.redisClient.Set(ctx, "latest_warning", string(jsonBytes), 0); err != nil {
							log.Printf("Error setting Redis key: %v", err)
						}

						if err := m.redisClient.Publish(ctx, "weather_alerts", string(jsonBytes)); err != nil {
							log.Printf("Error publishing to Redis: %v", err)
						}
					}
				}
			}
		}
	}

	m.lastCheckTime = currentTime
	return warnings, nil
}

func main() {
	cfg := config.Load()

	// Initialize Redis
	rClient := redis.NewClient(cfg.RedisHost, cfg.RedisPort, cfg.RedisDB)
	defer rClient.Close()

	ctx := context.Background()
	log.Println("Connecting to Redis...")
	if err := rClient.WaitReady(ctx, redis.DefaultWaitReadyMaxWait, redis.DefaultWaitReadyBackoff, log.Printf); err != nil {
		log.Fatalf("Redis unavailable after %v: %v", redis.DefaultWaitReadyMaxWait, err)
	}
	log.Println("Redis connected.")

	monitor := NewWeatherAlertMonitor(rClient)

	fmt.Println("Starting Weather Alert Monitor...")
	fmt.Println("Monitoring for severe thunderstorm alerts...")

	firstRun := true
	ticker := time.NewTicker(CheckInterval)
	defer ticker.Stop()

	// Initial check
	runCheck(monitor, &firstRun)

	for range ticker.C {
		runCheck(monitor, &firstRun)
	}
}

func runCheck(monitor *WeatherAlertMonitor, firstRun *bool) {
	alerts, err := monitor.getAlerts("", *firstRun)
	if err != nil {
		log.Printf("Error fetching alerts: %v\n", err)
		return
	}
	*firstRun = false

	if len(alerts) > 0 {
		fmt.Println("\nNew Severe Thunderstorm Alerts Found!")
		for _, a := range alerts {
			fmt.Println("\n" + strings.Repeat("=", 50))
			fmt.Printf("Event: %s\n", a.Event)
			fmt.Printf("Areas: %v\n", a.Areas)
			fmt.Printf("Time: %s\n", a.Time)
			fmt.Printf("Office: %s\n", a.Office)
			fmt.Printf("Max Hail Size: %v\n", a.HailSize)
			fmt.Println(strings.Repeat("=", 50))
		}
	}
}
