import os
from twilio.rest import Client
from datetime import datetime, timedelta
import redis
import json
import time


# Initialize Redis client
REDIS_HOST = os.getenv('REDIS_HOST', 'localhost')
REDIS_PORT = int(os.getenv('REDIS_PORT', 6379))
REDIS_DB = int(os.getenv('REDIS_DB', 0))
redis_client = redis.Redis(host=REDIS_HOST, port=REDIS_PORT, db=REDIS_DB)

# Twilio configuration
TWILIO_ACCOUNT_SID = os.getenv('TWILIO_ACCOUNT_SID')
TWILIO_AUTH_TOKEN = os.getenv('TWILIO_AUTH_TOKEN')
_raw_twilio_numbers = os.getenv('TWILIO_PHONE_NUMBERS', '')
try:
    _parsed = json.loads(_raw_twilio_numbers)
    TWILIO_PHONE_NUMBERS = _parsed[0] if isinstance(_parsed, list) and _parsed else _raw_twilio_numbers
except (json.JSONDecodeError, TypeError):
    TWILIO_PHONE_NUMBERS = _raw_twilio_numbers

# Dictionary of phone numbers with recipient names
NOTIFY_CONTACTS = {
    "Joey": "+17632348217",
    "Steve": "+14056559715",
    "Haylee": "+14054122989",
    "Matt": "+15409409827",
    "Luke": "+19495454510",
    "Chris": "+14056065314",
    "Chloe": "+17209876127",
    "John": "+14056559791"
    # Add more contacts as needed
}


# Time between notifications (2 hours)
NOTIFICATION_COOLDOWN = timedelta(hours=2)


def get_last_notification_time():
    """Get the timestamp of the last sent notification."""
    last_time = redis_client.get('last_notification_time')
    if last_time:
        return datetime.fromisoformat(last_time.decode('utf-8'))
    return None


def set_last_notification_time():
    """Set the current time as the last notification time."""
    current_time = datetime.now().isoformat()
    redis_client.set('last_notification_time', current_time)


def should_send_notification(warning_data):
    """Check if enough time has passed since the last notification."""
    last_time = get_last_notification_time()
    status = warning_data.get('status')

    if not last_time and status == 'Alert':
        return True

    if status == 'Update':
        try:
            if float(warning_data.get('hail_size') or 0) >= 1.75:
                return True
        except (TypeError, ValueError):
            pass  # if hail_size is bad, skip sending

    return datetime.now() - last_time >= NOTIFICATION_COOLDOWN


def format_warning_message(warning_data):
    """Format the warning data into a readable message."""
    areas = ', '.join(area.strip() + '' for area in warning_data['areas'])

    hail_size = warning_data.get('hail_size')
    hail_msg = ""
    status = warning_data.get('status')
    if hail_size:
        try:
            hail_value = float(hail_size)
            if hail_value >= 1.75:
                if hail_value >= 2.5:
                    hail_msg = f"⚠️ Very Large Hail: {hail_value:.2f}\" ⚠️"
                else:
                    hail_msg = f"⚠️ Large Hail: {hail_value:.2f}\" ⚠️"
            else:
                hail_msg = f"Hail: {hail_value:.2f}\""
        except ValueError:
            # If not numeric, ignore unrecognized hail sizes
            hail_msg = ""  # or optionally: hail_msg = f"Hail: {hail_size}"

    # Build final message
    message = (f"⚠️ Severe Thunderstorm ⚠️\n"
               f"Status: {status}\n"
               f"{hail_msg}\n"
               f"For: {areas}\n"
               f"Issued: {warning_data['time']}\n"
               f"From: {warning_data['office']}")

    print(message)  # Debugging output
    return message



def send_sms_notifications(warning_data):
    """Send SMS notifications to all numbers."""
    if not all([TWILIO_ACCOUNT_SID, TWILIO_AUTH_TOKEN, TWILIO_PHONE_NUMBERS]):
        print("Error: Twilio credentials not configured")
        return False

    hail_size = warning_data.get('hail_size')
    try:
        hail_value = float(hail_size) if hail_size else 0.0
    except ValueError:
        hail_value = 0.0  # fallback if hail_size is invalid or missing

    bypass_cooldown = hail_value >= 1.75

    if not bypass_cooldown and not should_send_notification(warning_data):
        print("Skipping notification: cooldown period not elapsed")
        return False

    if bypass_cooldown:
        print("Bypassing cooldown due to hail size >= 1.75\"")

    try:
        client = Client(TWILIO_ACCOUNT_SID, TWILIO_AUTH_TOKEN)
        message_text = format_warning_message(warning_data)

        for name, number in NOTIFY_CONTACTS.items():
            try:
                client.messages.create(
                    body=message_text,
                    from_=TWILIO_PHONE_NUMBERS,
                    to=number
                )
                print(f"Successfully sent SMS to {name} ({number})")
            except Exception as e:
                print(f"Error sending SMS to {name} ({number}): {str(e)}")

        set_last_notification_time()
        return True

    except Exception as e:
        print(f"Error initializing Twilio client: {str(e)}")
        return False


def handle_redis_message(message):
    """Handle incoming Redis pub/sub messages."""
    try:
        if message['type'] == 'message':
            data = json.loads(message['data'].decode('utf-8'))
            if data.get('type') == 'warning':
                print("\nReceived new severe thunderstorm warning")
                send_sms_notifications(data)
            else:
                print("\nReceived non-warning alert")
    except Exception as e:
        print(f"\nError processing Redis message: {str(e)}")

def send_test_message():
    """Send a test message to all contacts at startup."""
    if not all([TWILIO_ACCOUNT_SID, TWILIO_AUTH_TOKEN, TWILIO_PHONE_NUMBERS]):
        print("Error: Twilio credentials not configured")
        return False
        
    # Check if we've already sent a test today
    today_key = f"test_message_sent:{datetime.now().date().isoformat()}"
    try:
        if redis_client.get(today_key):
            print("Test message already sent today, skipping...")
            return False
    except redis.exceptions.ConnectionError as e:
        print(f"Redis connection failed: {e}")
        return False
        
    test_message = "⚠️ This is a test message from the Hail Alert System. ⚠️"

    try:
        client = Client(TWILIO_ACCOUNT_SID, TWILIO_AUTH_TOKEN)
        for name, number in NOTIFY_CONTACTS.items():
            try:
                client.messages.create(
                    body=test_message,
                    from_=TWILIO_PHONE_NUMBERS,
                    to=number
                )
                print(f"Test message sent to {name} ({number})")
            except Exception as e:
                print(f"Error sending test SMS to {name} ({number}): {str(e)}")
                
        # Mark test as sent for the day
        try:
            redis_client.set(today_key, "1", ex=86400)
        except redis.exceptions.ConnectionError as e:
            print(f"Redis failed while setting test key: {e}")  # expire after 1 day
        return True
    except Exception as e:
        print(f"Error initializing Twilio client for test message: {str(e)}")
        return False

def _make_redis_client():
    """Create a new Redis client (used for reconnects after connection reset)."""
    return redis.Redis(host=REDIS_HOST, port=REDIS_PORT, db=REDIS_DB)


def run_alert_monitor():
    """Main loop to monitor for new alerts using Redis pub/sub. Reconnects on connection errors."""
    global redis_client
    print("\nStarting SMS alert monitor...")
    print("Sending test SMS to all contacts...")
    send_test_message()
    print("Listening for new weather alerts in real-time...")

    while True:
        pubsub = None
        try:
            pubsub = redis_client.pubsub()
            pubsub.subscribe(**{'weather_alerts': handle_redis_message})

            while True:
                message = pubsub.get_message(timeout=1.0)
                if message is None:
                    continue
                if message['type'] == 'message':
                    handle_redis_message(message)

        except KeyboardInterrupt:
            print("\nStopping SMS alert monitor...")
            if pubsub:
                try:
                    pubsub.close()
                except Exception:
                    pass
            break
        except (redis.exceptions.ConnectionError, redis.exceptions.TimeoutError, OSError) as e:
            print(f"\nValkey/Redis connection error (will reconnect): {e}")
            if pubsub:
                try:
                    pubsub.close()
                except Exception:
                    pass
            redis_client = _make_redis_client()
            print("Reconnecting in 3 seconds...")
            time.sleep(3)
        except Exception as e:
            print(f"\nUnexpected error in alert monitor: {e}")
            if pubsub:
                try:
                    pubsub.close()
                except Exception:
                    pass
            print("Reconnecting in 5 seconds...")
            time.sleep(5)


if __name__ == '__main__':
    run_alert_monitor()
