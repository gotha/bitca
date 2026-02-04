# Default recipe to display help
default:
    @just --list

# Run the application
run:
    go run .

# Build the application
build:
    go build -o bitca .

