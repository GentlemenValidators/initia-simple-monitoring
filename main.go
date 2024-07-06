package main

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "io/ioutil"
    "log"
    "net/http"
    "os"
    "os/signal"
    "strconv"
    "strings"
    "syscall"
    "time"

    "github.com/joho/godotenv"
    "gopkg.in/yaml.v2"
)

const stateFile = "previous_state.yml"

type Config struct {
    RPCs  []string
    Node  string
    Alert AlertConfig
}

type AlertConfig struct {
    Level1 int
    Level2 int
    Level3 int
}

type TelegramConfig struct {
    BotToken string
    ChatID   string
}

type State struct {
    PreviousHeightDiff int `yaml:"previous_height_diff"`
    LastAlertLevel     int `yaml:"last_alert_level"`
}

func loadConfig() (*Config, error) {
    err := godotenv.Load(".env")
    if err != nil {
        return nil, fmt.Errorf("error loading .env file: %w", err)
    }

    rpcUrls := strings.Split(os.Getenv("RPC_URLS"), ",")
    nodeUrl := os.Getenv("NODE_URL")

    level1, err := strconv.Atoi(os.Getenv("LEVEL_1"))
    if err != nil {
        return nil, fmt.Errorf("error converting LEVEL_1: %w", err)
    }

    level2, err := strconv.Atoi(os.Getenv("LEVEL_2"))
    if err != nil {
        return nil, fmt.Errorf("error converting LEVEL_2: %w", err)
    }

    level3, err := strconv.Atoi(os.Getenv("LEVEL_3"))
    if err != nil {
        return nil, fmt.Errorf("error converting LEVEL_3: %w", err)
    }

    config := &Config{
        RPCs: rpcUrls,
        Node: nodeUrl,
        Alert: AlertConfig{
            Level1: level1,
            Level2: level2,
            Level3: level3,
        },
    }
    return config, nil
}

func loadTelegramConfig() (*TelegramConfig, error) {
    err := godotenv.Load(".env")
    if err != nil {
        return nil, fmt.Errorf("error loading .env file: %w", err)
    }

    botToken := os.Getenv("BOT_TOKEN")
    chatID := os.Getenv("CHAT_ID")

    config := &TelegramConfig{
        BotToken: botToken,
        ChatID:   chatID,
    }
    return config, nil
}

func fetchStatus(url string) (int, error) {
    resp, err := http.Get(fmt.Sprintf("%s/status", url))
    if err != nil {
        return 0, fmt.Errorf("failed to get status from %s: %w", url, err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return 0, fmt.Errorf("unexpected status code %d from %s", resp.StatusCode, url)
    }

    var result struct {
        Result struct {
            SyncInfo struct {
                LatestBlockHeight string `json:"latest_block_height"`
            } `json:"sync_info"`
        } `json:"result"`
    }
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return 0, fmt.Errorf("failed to decode response from %s: %w", url, err)
    }

    blockHeight, err := strconv.Atoi(result.Result.SyncInfo.LatestBlockHeight)
    if err != nil {
        return 0, fmt.Errorf("failed to convert block height to int: %w", err)
    }

    return blockHeight, nil
}

func checkStatus(rpcURLs []string) ([]int, error) {
    heights := make([]int, 0, len(rpcURLs))
    for _, rpc := range rpcURLs {
        height, err := fetchStatus(rpc)
        if err != nil {
            log.Printf("Error fetching status: %v", err)
            continue
        }
        heights = append(heights, height)
    }
    return heights, nil
}

func compareWithNode(nodeURL string, highestHeight int) (int, error) {
    nodeHeight, err := fetchStatus(nodeURL)
    if err != nil {
        return 0, err
    }
    return highestHeight - nodeHeight, nil
}

func sendTelegramMessage(token, chatID, message string) error {
    url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token)
    payload := map[string]string{
        "chat_id": chatID,
        "text":    message,
    }

    jsonPayload, err := json.Marshal(payload)
    if err != nil {
        return fmt.Errorf("failed to marshal payload: %w", err)
    }

    resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonPayload))
    if err != nil {
        return fmt.Errorf("failed to send message: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        bodyBytes, _ := ioutil.ReadAll(resp.Body)
        bodyString := string(bodyBytes)
        return fmt.Errorf("unexpected status code %d from Telegram API: %s", resp.StatusCode, bodyString)
    }
    return nil
}

func loadPreviousState() (*State, error) {
    if _, err := os.Stat(stateFile); os.IsNotExist(err) {
        return &State{}, nil
    }

    data, err := ioutil.ReadFile(stateFile)
    if err != nil {
        return nil, fmt.Errorf("failed to read state file: %w", err)
    }

    var state State
    if err := yaml.Unmarshal(data, &state); err != nil {
        return nil, fmt.Errorf("failed to unmarshal state file: %w", err)
    }
    return &state, nil
}

func savePreviousState(heightDiff, alertLevel int) error {
    state := State{
        PreviousHeightDiff: heightDiff,
        LastAlertLevel:     alertLevel,
    }

    data, err := yaml.Marshal(&state)
    if err != nil {
        return fmt.Errorf("failed to marshal state: %w", err)
    }

    if err := ioutil.WriteFile(stateFile, data, 0644); err != nil {
        return fmt.Errorf("failed to write state file: %w", err)
    }
    return nil
}

func determineAlertLevel(heightDiff int, alertConfig AlertConfig) int {
    switch {
    case heightDiff >= alertConfig.Level3:
        return 3
    case heightDiff >= alertConfig.Level2:
        return 2
    case heightDiff >= alertConfig.Level1:
        return 1
    default:
        return 0
    }
}

func alert(heightDiff int, alertConfig AlertConfig, telegramConfig *TelegramConfig, previousState *State) error {
    lastAlertLevel := previousState.LastAlertLevel
    currentAlertLevel := determineAlertLevel(heightDiff, alertConfig)
    message := ""

    switch {
    case currentAlertLevel > lastAlertLevel:
        message = fmt.Sprintf("Alert Level %d: Block height difference is %d blocks!", currentAlertLevel, heightDiff)
    case currentAlertLevel < lastAlertLevel:
        message = fmt.Sprintf("Alert Level Dropping to %d: Block height difference is %d blocks!", currentAlertLevel, heightDiff)
    default:
        return nil // No change in alert level, no need to send a message
    }

    if message != "" {
        log.Println(message)
        if err := sendTelegramMessage(telegramConfig.BotToken, telegramConfig.ChatID, message); err != nil {
            return err
        }
    }

    // Save the new state
    return savePreviousState(heightDiff, currentAlertLevel)
}

func periodicCheck(ctx context.Context, telegramConfig *TelegramConfig, config *Config) {
    nodeDownTimer := 0
    notificationTimer := 0
    ticker := time.NewTicker(15 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            log.Println("Starting new check cycle.")
            heights, err := checkStatus(config.RPCs)
            if err != nil {
                log.Printf("Error checking status: %v", err)
                continue
            }

            if len(heights) == 0 {
                nodeDownTimer += 15
                if nodeDownTimer >= 60 {
                    notificationTimer += 15
                    if notificationTimer >= 60 {
                        if err := sendTelegramMessage(telegramConfig.BotToken, telegramConfig.ChatID, "None of the RPC endpoints can be reached for more than 3 minutes!"); err != nil {
                            log.Printf("Error sending Telegram message: %v", err)
                        }
                        notificationTimer = 0
                    }
                }
                continue
            }

            highestHeight := max(heights)
            heightDiff, err := compareWithNode(config.Node, highestHeight)
            if err != nil {
                log.Printf("Error comparing with node: %v", err)
                continue
            }

            previousState, err := loadPreviousState()
            if err != nil {
                log.Printf("Error loading previous state: %v", err)
                continue
            }

            if err := alert(heightDiff, config.Alert, telegramConfig, previousState); err != nil {
                log.Printf("Error sending alert: %v", err)
            }

            nodeDownTimer = 0
            notificationTimer = 0
            log.Printf("Current block height difference: %d blocks", heightDiff)
        }
    }
}

func max(slice []int) int {
    maxValue := slice[0]
    for _, value := range slice[1:] {
        if value > maxValue {
            maxValue = value
        }
    }
    return maxValue
}

func handleUpdates(telegramConfig *TelegramConfig, messageSent *bool) {
    url := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates", telegramConfig.BotToken)
    resp, err := http.Get(url)
    if err != nil {
        log.Printf("Failed to get updates: %v", err)
        return
    }
    defer resp.Body.Close()

    var result struct {
        Result []struct {
            Message struct {
                Text string `json:"text"`
                Chat struct {
                    ID int64 `json:"id"`
                } `json:"chat"`
            } `json:"message"`
        } `json:"result"`
    }

    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        log.Printf("Failed to decode response: %v", err)
        return
    }

    for _, update := range result.Result {
        if update.Message.Text == "/start" && !*messageSent {
            message := "Monitoring has started!"
            if err := sendTelegramMessage(telegramConfig.BotToken, strconv.FormatInt(update.Message.Chat.ID, 10), message); err != nil {
                log.Printf("Failed to send message: %v", err)
            } else {
                *messageSent = true
            }
        }
    }
}

func main() {
    log.SetFlags(log.LstdFlags | log.Lshortfile)
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    config, err := loadConfig()
    if err != nil {
        log.Fatalf("Failed to load config: %v", err)
    }

    telegramConfig, err := loadTelegramConfig()
    if err != nil {
        log.Fatalf("Failed to load telegram config: %v", err)
    }

    go periodicCheck(ctx, telegramConfig, config)

    // Track if the message has been sent
    messageSent := false

    // Handle Telegram updates in a separate goroutine
    go func() {
        for {
            handleUpdates(telegramConfig, &messageSent)
            time.Sleep(2 * time.Second)
        }
    }()

    // Handle graceful shutdown
    sig := make(chan os.Signal, 1)
    signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
    <-sig
    cancel()
}
