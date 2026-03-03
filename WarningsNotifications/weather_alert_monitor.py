import requests
import time
from datetime import datetime, timezone
import json
from dateutil import parser
import os
import redis
import pytz

# Initialize Redis client
REDIS_HOST = os.getenv('REDIS_HOST', 'localhost')
REDIS_PORT = int(os.getenv('REDIS_PORT', 6379))
REDIS_DB = int(os.getenv('REDIS_DB', 0))
redis_client = redis.Redis(host=REDIS_HOST, port=REDIS_PORT, db=REDIS_DB)

# Retry config for transient connection errors (e.g. connection reset by peer)
REDIS_RETRY_ATTEMPTS = 3
REDIS_RETRY_DELAY = 1.0


def _redis_operation_with_retry(operation, *args, **kwargs):
    """Run a Redis operation with retries on connection errors."""
    last_error = None
    client = redis_client
    for attempt in range(REDIS_RETRY_ATTEMPTS):
        try:
            return operation(client, *args, **kwargs)
        except (redis.exceptions.ConnectionError, redis.exceptions.TimeoutError, OSError) as e:
            last_error = e
            if attempt < REDIS_RETRY_ATTEMPTS - 1:
                time.sleep(REDIS_RETRY_DELAY * (attempt + 1))
    if last_error:
        print(f"Redis operation failed after {REDIS_RETRY_ATTEMPTS} attempts: {last_error}")
    return None

class WeatherAlertMonitor:
    def __init__(self):
        self.base_url = "https://api.weather.gov"
        self.headers = {
            "User-Agent": "WeatherAlertMonitor/1.0 (your-email@example.com)",
            "Accept": "application/geo+json"
        }
        self.last_check_time = datetime.now(timezone.utc)
        self.seen_ids = set()  # Track seen alert IDs

    def format_warning(self, properties):
        areas = [area.strip() for area in properties.get("areaDesc", "").split(",")]
        description = properties.get("description", "")
        max_hail_list = properties.get("parameters", {}).get("maxHailSize", [])
        hail_size = max_hail_list[0] if max_hail_list else None

        warning = {
            "type": "warning",
            "event": "Severe Thunderstorm Warning",
            "areas": areas,
            "description": description,
            "instructions": properties.get("instruction",
                                           "TAKE COVER NOW! Move to an interior room on the lowest floor of a sturdy building."),
            "office": properties.get("senderName", "NWS"),
            "time": datetime.now(pytz.timezone('US/Central')).strftime('%Y-%m-%d %I:%M:%S %p'),
            "hail_size": hail_size,
            "status": properties.get("messageType", "")
        }

        return warning

    def get_alerts(self, area=None, force_first_send=False):
        try:
            current_time = datetime.now(timezone.utc)  # Avoid race condition
            url = f"{self.base_url}/alerts/active"
            if area:
                url += f"/area/{area}"

            response = requests.get(url, headers=self.headers)
            response.raise_for_status()
            data = response.json()

            alerts = []
            for feature in data.get("features", []):
                properties = feature.get("properties", {})
                event = properties.get("event", "")
                alert_id = feature.get("id", "")

                if "Severe Thunderstorm Warning" in event:
                    sent_time = parser.parse(properties.get("sent", ""))

                    from datetime import timedelta
                    if (force_first_send or (sent_time >= self.last_check_time - timedelta(
                            seconds=60)) and alert_id not in self.seen_ids):

                        self.seen_ids.add(alert_id)
                        warning = self.format_warning(properties)
                        alerts.append(warning)

                        #if properties.get("messageType", "") in ['Alert']:
                        if properties.get("messageType", "") in ['Alert', 'Update']:
                            _redis_operation_with_retry(lambda c: c.set('latest_warning', json.dumps(warning)))
                            _redis_operation_with_retry(lambda c: c.publish('weather_alerts', json.dumps(warning)))

            self.last_check_time = current_time  # Only update after processing
            return alerts

        except requests.exceptions.RequestException as e:
            print(f"Error fetching alerts: {e}")
            return []

def main():
    monitor = WeatherAlertMonitor()
    check_interval = 30
    first_run = True

    print("Starting Weather Alert Monitor...")
    print("Monitoring for severe thunderstorm alerts...")

    while True:
        try:
            alerts = monitor.get_alerts(force_first_send=first_run)
            first_run = False

            if alerts:
                print("\nNew Severe Thunderstorm Alerts Found!")
                for alert in alerts:
                    print("\n" + "=" * 50)
                    print(f"Event: {alert['event']}")
                    print(f"Areas: {alert['areas']}")
                    print(f"Time: {alert['time']}")
                    print(f"Office: {alert['office']}")
                    print(f"Max Hail Size: {alert['hail_size']}")
                    print("=" * 50)

            time.sleep(check_interval)

        except KeyboardInterrupt:
            print("\nStopping Weather Alert Monitor...")
            break
        except Exception as e:
            print(f"An error occurred: {e}")
            time.sleep(check_interval)

if __name__ == "__main__":
    main()
