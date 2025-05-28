#!/bin/sh

# Validate required environment variables
: "${DB_HOST:?Environment variable DB_HOST is required}"
: "${DB_PORT:?Environment variable DB_PORT is required}"
: "${DB_USER:?Environment variable DB_USER is required}"
: "${DB_NAME:?Environment variable DB_NAME is required}"

# Wait for database to be ready
echo "Waiting for database to be ready..."
until nc -z "$DB_HOST" "$DB_PORT"; do
  echo "Database is not ready yet. Waiting 2 seconds..."
  sleep 2
done
echo "Database is ready!"

# Run Atlas migrations with explicit database URL
echo "Running Atlas migrations..."
atlas migrate apply \
  --dir "file://migrations" \
  --url "postgres://$DB_USER:$DB_PASSWORD@$DB_HOST:$DB_PORT/$DB_NAME?sslmode=disable"

# Check if migrations succeeded
# Get exit status of last command
if [ $? -eq 0 ]; then
  echo "Atlas migrations completed successfully!"
else
  echo "Atlas migrations failed!"
  exit 1
fi
