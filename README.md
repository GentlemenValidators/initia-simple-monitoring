# Go Blockchain Monitoring Application

This is a Go application that monitors the block height difference between a node and a set of RPC endpoints. Alerts are sent to a Telegram bot when certain thresholds are exceeded.

## Prerequisites

- Docker
- Docker Compose

## Setup

1. Clone the repository:

    ```sh
    git clone https://github.com/your-repo/go-blockchain-monitoring.git
    cd go-blockchain-monitoring
    ```

2. Create a `.env` file in the project root directory and add the necessary environment variables:

    ```sh
    RPC_URLS=http://rpc1,http://rpc2
    NODE_URL=http://node
    LEVEL_1=5
    LEVEL_2=10
    LEVEL_3=15
    BOT_TOKEN=your_telegram_bot_token
    CHAT_ID=your_telegram_chat_id
    ```

    Replace `your_telegram_bot_token` and `your_telegram_chat_id` with your actual Telegram bot token and chat ID. Adjust the other environment variables as needed.

3. Create an empty `previous_state.yml` file in the project root directory:

    ```sh
    touch previous_state.yml
    ```

## Docker Setup

1. Build the Docker image:

    ```sh
    docker build -t go-blockchain-monitoring .
    ```

2. Run the Docker container:

    ```sh
    docker run --env-file .env -v $(pwd)/previous_state.yml:/root/previous_state.yml go-blockchain-monitoring
    ```

## Docker Compose Setup

1. Ensure you have the `docker-compose.yml` file in the project root directory.

2. Build and start the application using Docker Compose:

    ```sh
    docker-compose up --build
    ```

    This command will build the Docker image and start the container.

3. To stop the application:

    ```sh
    docker-compose down
    ```

## Application Structure

- `main.go`: Main application file.
- `Dockerfile`: Dockerfile for building the application image.
- `docker-compose.yml`: Docker Compose configuration file.
- `.env`: Environment variables file (you need to create this).
- `previous_state.yml`: File to persist the previous state (you need to create this).

## Contributing

If you find any issues or have suggestions for improvements, feel free to open an issue or submit a pull request.
