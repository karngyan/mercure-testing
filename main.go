package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/r3labs/sse/v2"
)

var (
	totalConnections atomic.Int32
	baseURL          = ""
	secret           = ""
)

func main() {
	fmt.Println("Starting SSE Load Test")

	// Generate a subscriber JWT
	subscriberJWT := generateSubscriberJWT(secret)

	// Create a context that listens for interrupt signals
	ctx, cancel := context.WithCancel(context.Background())
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-signalChan
		fmt.Println("\nShutting down...")
		cancel()
	}()

	// Run an initial test with one connection
	fmt.Println("Testing with one connection initially...")
	runLoadTest(ctx, 1, baseURL, subscriberJWT)

	reader := bufio.NewReader(os.Stdin)
	for {
		select {
		case <-ctx.Done():
			return
		default:
			fmt.Printf("\nCurrent total connections: %d\n", totalConnections.Load())
			fmt.Print("Enter the number of additional connections to add (e.g., 10000): ")

			input, _ := reader.ReadString('\n')
			input = strings.TrimSpace(input)
			additionalConnections, err := strconv.Atoi(input)
			if err != nil {
				fmt.Println("Invalid input, please enter a valid number.")
				continue
			}

			fmt.Printf("Adding %d connections...\n", additionalConnections)
			runLoadTest(ctx, additionalConnections, baseURL, subscriberJWT)
		}
	}
}

func runLoadTest(ctx context.Context, numConnections int, baseURL, subscriberJWT string) {
	for i := 1; i <= numConnections; i++ {
		go func(connectionID int) {
			topic := fmt.Sprintf("test-topic-%d", connectionID)
			url := fmt.Sprintf("%s?topic=%s", baseURL, topic)

			client := sse.NewClient(url)

			// Add Authorization header
			client.Headers["Authorization"] = "Bearer " + subscriberJWT

			go func() {
				err := client.SubscribeRaw(func(msg *sse.Event) {
					fmt.Printf("Connection %d received message: %s\n", connectionID, string(msg.Data))
				})
				if err != nil {
					fmt.Printf("Connection %d failed: %v\n", connectionID, err)
					return
				}
			}()

			fmt.Printf("Connection %d to topic '%s' established.\n", connectionID, topic)
			totalConnections.Add(1)
			fmt.Printf("Current total connections: %d\n", totalConnections.Load())

			// Wait until context is canceled
			<-ctx.Done()
			fmt.Printf("Connection %d shutting down.\n", connectionID)
			totalConnections.Add(-1)
		}(i)
	}
}

func generateSubscriberJWT(secret string) string {
	// JWT claims for subscribing to topics
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"mercure": map[string]interface{}{
			"subscribe": []string{"*"}, // Allow subscribing to all topics
		},
		"exp": time.Now().Add(time.Hour * 24).Unix(), // Token expiration (24 hours)
	})

	// Sign the token with the secret key
	tokenString, err := token.SignedString([]byte(secret))
	if err != nil {
		fmt.Printf("Failed to generate JWT: %v\n", err)
		os.Exit(1)
	}

	return tokenString
}
