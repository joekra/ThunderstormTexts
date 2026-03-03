import os
import subprocess
import sys
from pathlib import Path


def load_env_if_present():
    """Load environment variables from .env file if it exists. Does not exit if missing."""
    env_path = Path(__file__).parent / '.env'

    if env_path.exists():
        env_vars = {}
        with open(env_path) as f:
            for line in f:
                line = line.strip()
                if line and not line.startswith('#'):
                    if line.startswith('export '):
                        line = line[7:]
                    key, value = line.split('=', 1)
                    env_vars[key.strip()] = value.strip()
        os.environ.update(env_vars)
        return True
    return False


def check_required_env():
    """Verify required Twilio variables are set. Exits if any are missing."""
    required_vars = [
        'TWILIO_ACCOUNT_SID',
        'TWILIO_AUTH_TOKEN',
        'TWILIO_PHONE_NUMBERS',
    ]
    missing_vars = [var for var in required_vars if not os.environ.get(var)]
    if missing_vars:
        print("Error: Missing required environment variables:", ', '.join(missing_vars))
        print("Set them in the environment or create a .env file (see .env.example).")
        sys.exit(1)

    print("\nEnvironment configuration:")
    for var in required_vars:
        if var.startswith('TWILIO'):
            value = os.environ[var]
            masked_value = value[:4] + '*' * (len(value) - 4)
            print(f"{var}: {masked_value}")
        else:
            print(f"{var}: {os.environ[var]}")


def run_sms_notifier():
    """Run the SMS notifier with the loaded environment."""
    sms_notifier_path = Path(__file__).parent / 'sms_notifier.py'

    if not sms_notifier_path.exists():
        print("Error: sms_notifier.py not found!")
        sys.exit(1)

    print("\nStarting SMS notifier...")
    try:
        # Run sms_notifier.py with the current environment
        process = subprocess.run([sys.executable, str(sms_notifier_path)], check=True)
    except KeyboardInterrupt:
        print("\nSMS notifier stopped by user")
    except subprocess.CalledProcessError as e:
        print(f"\nError running SMS notifier: {e}")
        sys.exit(1)


if __name__ == '__main__':
    if load_env_if_present():
        print("Loaded variables from .env")
    else:
        print("No .env file; using environment variables")
    check_required_env()
    run_sms_notifier() 