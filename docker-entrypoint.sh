#!/bin/sh
set -e

args="serve -host 0.0.0.0 -port ${PORT:-8080} -no-browser"

if [ -n "$RAILWAY_PUBLIC_DOMAIN" ]; then
  args="$args -public-url https://$RAILWAY_PUBLIC_DOMAIN"
fi

exec agentsview $args
