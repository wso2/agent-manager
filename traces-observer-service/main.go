// Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/wso2/ai-agent-management-platform/traces-observer-service/config"
	"github.com/wso2/ai-agent-management-platform/traces-observer-service/controllers"
	"github.com/wso2/ai-agent-management-platform/traces-observer-service/handlers"
	"github.com/wso2/ai-agent-management-platform/traces-observer-service/middleware"
	"github.com/wso2/ai-agent-management-platform/traces-observer-service/opensearch"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	log.Printf("Starting tracing service on port %d", cfg.Server.Port)

	// Initialize OpenSearch client
	osClient, err := opensearch.NewClient(&cfg.OpenSearch)
	if err != nil {
		// log.Fatalf internally calls os.Exit(1)
		log.Fatalf("Failed to create OpenSearch client: %v", err)
	}

	// Initialize service
	tracingController := controllers.NewTracingController(osClient)

	// Initialize handlers
	handler := handlers.NewHandler(tracingController)

	// Setup routes
	mux := http.NewServeMux()
	mux.HandleFunc("/api/traces", handler.GetTraceOverviews)
	mux.HandleFunc("/api/trace", handler.GetTraceByIdAndService)
	mux.HandleFunc("/health", handler.Health)

	// Apply CORS middleware
	corsConfig := middleware.DefaultCORSConfig()
	corsHandler := middleware.CORS(corsConfig)(mux)

	// Create server
	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:      corsHandler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	// Start server in a goroutine
	go func() {
		log.Printf("Server listening on :%d", cfg.Server.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited")
}
