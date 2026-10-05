#!/bin/sh

# Certificate auto-renewal: acme.sh (installed by the panel's SSL menu) relies
# on a root crontab entry, but the crontab is lost when the container is
# recreated and crond was never started. Re-register the job and run crond so
# renewals actually fire; mount /root/.acme.sh as a volume to keep acme state.
if [ -f /root/.acme.sh/acme.sh ]; then
    /root/.acme.sh/acme.sh --install-cronjob >/dev/null 2>&1
    crond
fi

# Run x-ui
exec /app/x-ui
