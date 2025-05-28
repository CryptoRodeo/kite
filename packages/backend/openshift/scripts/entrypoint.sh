#!/bin/sh

# Validate required environment variables
: "${DB_HOST:?Environment variable DB_HOST is required}"
: "${DB_PORT:?Environment variable DB_PORT is required}"

# Wait for database to be ready
echo "Waiting for database to be ready..."
until nc -z "$DB_HOST" "$DB_PORT"; do
  echo "Database is not ready yet. Waiting 2 seconds..."
  sleep 2
done
echo "Database is ready!"

# Note: Migrations should be run externally or in a separate init container
echo "Skipping migrations (run them externally before starting the app)"

# Start the main application
echo "Starting server..."
exec ./server
