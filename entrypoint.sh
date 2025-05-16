#!/bin/bash
set -e

# Wait for the database to be ready
echo "Waiting for database..."
until nc -z db 5432; do
  sleep 1
done
echo "Database is ready!"

# Run the database migrations and seeding in development
if [ "$NODE_ENV" = "development" ]; then
  echo "Running database migrations"
  cd /apps/packages/backend
  yarn db:migrate:dev

  echo "Seeding the database..."
  yarn db:seed
fi

# Start the app
echo "Starting application..."
exec "$@"
