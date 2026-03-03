# Thunderstorm Alerts (Go Implementation)

This project is a rewrite of the original Python-based thunderstorm alert system in Go. It monitors National Weather Service (NWS) alerts for severe thunderstorms and sends SMS notifications via Twilio when specific criteria are met (e.g., large hail).

## Features

- **Real-time Monitoring**: Polls the NWS API for active Severe Thunderstorm Warnings.
- **Hail Size Filtering**: specific alerts based on hail size (e.g., sends immediately if hail >= 1.75").
- **Smart Notifications**: Implements cooldown periods to prevent spamming, but bypasses cooldowns for significant updates (very large hail).
- **Redis Integration**: Uses Redis for state management (last notification time) and Pub/Sub for communication between the monitor and notifier services.
- **Twilio Integration**: Sends SMS alerts to a configured list of contacts.

## Project Structure

```
.
├── go_app/
│   ├── cmd/
│   │   ├── monitor/    # Service that polls NWS and publishes alerts to Redis
│   │   └── notifier/   # Service that subscribes to Redis and sends SMS
│   ├── internal/       # Shared internal packages (config, redis, alert models)
│   └── go.mod          # Go module definition
├── Makefile            # Build and run commands
└── Dockerfile          # Docker build definition
```

## Prerequisites

- Go 1.25 or higher
- Redis server running (default: localhost:6379)
- Twilio Account (Account SID, Auth Token, and a verified phone number)

## Configuration

The application is configured via environment variables. You can set these in your shell or use a `.env` file (if you add `godotenv` loading logic, which is included in the config package).

| Variable | Description | Default |
|----------|-------------|---------|
| `REDIS_HOST` | Redis server hostname | `localhost` |
| `REDIS_PORT` | Redis server port | `6379` |
| `REDIS_DB` | Redis database index | `0` |
| `TWILIO_ACCOUNT_SID` | Your Twilio Account SID | Required |
| `TWILIO_AUTH_TOKEN` | Your Twilio Auth Token | Required |
| `TWILIO_PHONE_NUMBERS` | Sender phone number (or JSON array) | Required |
| `NOTIFY_CONTACTS` | JSON map of contacts (e.g., `{"Name": "+1234567890"}`) | Optional |

## Building and Running

### Using Makefile

A `Makefile` is provided for convenience.

1. **Build Binaries**:
   ```bash
   make build
   ```
   This will create `monitor` and `notifier` binaries in the `bin/` directory.

2. **Run Monitor**:
   ```bash
   make run-monitor
   ```

3. **Run Notifier**:
   ```bash
   make run-notifier
   ```

### Running with Docker

1. **Build Image**:
   ```bash
   docker build -t thunderstorm-alerts .
   ```

2. **Run Container**:
   ```bash
   docker run -d \
     -e TWILIO_ACCOUNT_SID=your_sid \
     -e TWILIO_AUTH_TOKEN=your_token \
     -e TWILIO_PHONE_NUMBERS=+1234567890 \
     -e REDIS_HOST=host.docker.internal \
     thunderstorm-alerts
   ```
   *Note: Ensure the container can access your Redis instance.*

## Logic Overview

1. **Monitor**:
   - Polls `https://api.weather.gov/alerts/active` every 30 seconds.
   - Filters for "Severe Thunderstorm Warning".
   - Publishes new or updated warnings to the `weather_alerts` Redis channel.

2. **Notifier**:
   - Subscribes to `weather_alerts` channel.
   - Checks if the alert warrants a notification:
     - Always sends if status is "Alert" and no notification sent recently.
     - Bypasses cooldown if hail size is $\ge 1.75$ inches.
     - Respects a 2-hour cooldown for other updates.
   - Sends SMS via Twilio to configured contacts (via `NOTIFY_CONTACTS`).