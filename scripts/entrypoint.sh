#!/bin/bash
set -e

# Wait for the database to be ready
echo "Waiting for database..."
until nc -z db 5432; do
  sleep 1
done
echo "Database is ready!"

# Working directory is already /opt/app-root/src
cd packages/backend

# Always run migrations to ensure schema is up to date
echo "Running database migrations"
npx prisma migrate deploy

# Only seed in development mode
if [[ "$NODE_ENV" == "development" ]]; then
  echo "Checking if database needs seeding..."

  # Use psql directly to check if table has records
  # Hide any errors
  # If the command fails, just output "0"
  RECORD_COUNT=$(PGPASSWORD=postgres psql -h db -U kite -d issuesdb -t -c "SELECT COUNT(*) FROM \"Issue\";" 2>/dev/null || echo "0")
  # Take the record count we got, delete extra spaces from DB output
  RECORD_COUNT=$(echo $RECORD_COUNT | tr -d ' ')

  # Check if the coutn is exactly zero or if it's not a number
  if [[ $RECORD_COUNT == "0" ]] || [[ ! $RECORD_COUNT =~ ^[0-9]+$ ]]; then
    echo "Seeding the database... (count was: $RECORD_COUNT)"
    npx prisma db seed
  else
    echo "Database already has data ($RECORD_COUNT records), skipping seed"
  fi
fi

# Start the application
echo "Starting application..."
# Run whatever is set for CMD in the Containerfile
exec "$@"
