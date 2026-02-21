# GopherDrive 2.0 🐹

> High-Performance Concurrent File Processing System

GopherDrive is a production-grade concurrent file processing engine
built with **Go**, engineered for high-throughput, low-latency
workloads. The system ingests file uploads via a REST API, securely
persists them using stream-based I/O, and performs asynchronous
background processing through a bounded worker pool architecture.

Version **2.0** introduces enhanced metadata extraction, cryptographic
integrity verification, and a hybrid **REST + gRPC** design for improved
scalability and performance.

------------------------------------------------------------------------

## 🚀 Core Features

### High-Concurrency Architecture

-   **Bounded Worker Pool**\
    A fixed pool of workers processes jobs via buffered channels,
    ensuring controlled parallelism and preventing resource exhaustion.

-   **Asynchronous Processing Pipeline**\
    Upload requests return immediately while compute-intensive
    operations execute safely in the background.

-   **Graceful Shutdown Mechanism**\
    Proper handling of OS signals (`SIGINT`, `SIGTERM`) guarantees job
    completion and safe resource cleanup.

------------------------------------------------------------------------

### Robust & Safe File Handling

-   **Stream-Based I/O**\
    Files are processed using `io.Copy` and buffered readers,
    maintaining constant memory usage regardless of file size.

-   **Atomic File Writes**\
    Temporary file staging followed by atomic renaming eliminates
    partial-write corruption risks.

-   **Security Safeguards**

    -   UUID-based filenames (collision-safe)
    -   Path traversal protection
    -   File size limit enforcement (32MB)

------------------------------------------------------------------------

### Rich Content Analysis (v2.0)

-   **SHA-256 Hashing**\
    Cryptographic integrity verification for every uploaded file.

-   **MIME Type Detection**\
    Byte-level content inspection for accurate classification.

-   **Deep Metadata Extraction**

    -   **Images** → Width × Height
    -   **Text Files** → Word & Line Counts

-   **Flexible Metadata Storage**\
    Metadata is stored as JSON within MySQL for schema adaptability.

------------------------------------------------------------------------

### Hybrid API Design

-   **REST Gateway** → Public interaction layer\
-   **gRPC Layer** → High-performance internal database operations

------------------------------------------------------------------------

## 🛠 System Architecture

``` mermaid
graph LR
    User -->|POST /files| REST[REST API]
    REST -->|Stream| Disk[Local Storage]
    REST -->|Submit Job| Queue[Job Channel]
    REST -->|gRPC Register| DB[(MySQL)]
    
    subgraph WorkerPool[Worker Pool - Goroutines]
        Queue --> W1
        Queue --> W2
        Queue --> WN
    end
    
    WN --> Analyzer[Hasher & Metadata Analyzer]
    WN -->|gRPC Update| DB
```

------------------------------------------------------------------------

## 📦 Installation & Setup

### Prerequisites

-   Go 1.21+
-   MySQL 8.0+

------------------------------------------------------------------------

### Database Initialization

``` sql
CREATE DATABASE gopherdrive;
USE gopherdrive;

CREATE TABLE files (
    id         VARCHAR(36)  PRIMARY KEY,
    hash       VARCHAR(64)  NOT NULL DEFAULT '',
    size       BIGINT       NOT NULL DEFAULT 0,
    status     VARCHAR(20)  NOT NULL DEFAULT 'pending',
    file_path  VARCHAR(512) NOT NULL,
    created_at TIMESTAMP    DEFAULT CURRENT_TIMESTAMP,
    metadata   JSON
);
```

------------------------------------------------------------------------

### Running the Server

``` bash
# Optional cleanup
rm -rf data/*

# Start server (adjust credentials if needed)
DB_DSN="root:password@tcp(127.0.0.1:3306)/gopherdrive?parseTime=true" go run ./cmd/server/
```

**Note:**\
The `parseTime=true` flag is required for proper timestamp handling.

------------------------------------------------------------------------

## 🖥 Dashboard & API

### Web Dashboard

Access via:

http://localhost:8080

Features:

✔ Real-time upload monitoring\
✔ Drag-and-drop interface\
✔ Metadata visualization

------------------------------------------------------------------------

### API Endpoints

#### Upload File

`POST /files`

**Request:**\
`multipart/form-data` → `file`

**Response:**

``` json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "pending"
}
```

------------------------------------------------------------------------

#### Retrieve File Details

`GET /files/{id}`

``` json
{
  "id": "550e8400...",
  "status": "completed",
  "hash": "a1b2c3...",
  "size": 1024,
  "created_at": "2026-02-19T10:00:00Z",
  "metadata": {
    "mime_type": "image/png",
    "width": 800,
    "height": 600
  }
}
```

------------------------------------------------------------------------

#### Health Check

`GET /healthz`

Verifies:

✔ Database connectivity\
✔ Disk writeability

------------------------------------------------------------------------

## ✅ System Validation

GopherDrive has been validated for:

✔ Concurrent request handling\
✔ Worker pool stability under load\
✔ Cryptographic hash correctness\
✔ Atomic persistence reliability\
✔ Graceful shutdown safety

------------------------------------------------------------------------

## 🏃‍♂️ Manual Execution Guide

### Start Database

``` bash
mysql -u root -e "CREATE DATABASE IF NOT EXISTS gopherdrive;"
mysql -u root gopherdrive < schema/init.sql
```

------------------------------------------------------------------------

### Launch Server

``` bash
DB_DSN="root:mypassword@tcp(127.0.0.1:3306)/gopherdrive?parseTime=true" go run ./cmd/server/
```

------------------------------------------------------------------------

### Quick Test

``` bash
curl -F "file=@README.md" http://localhost:8080/files
```

------------------------------------------------------------------------

## 📂 Project Structure

    cmd/
     └── server/        # Application entry point

    internal/
     ├── grpcserver/    # gRPC service layer
     ├── hasher/        # SHA-256 & metadata logic
     ├── repository/    # MySQL data access layer
     ├── restapi/       # REST handlers
     └── worker/        # Concurrent worker pool

    proto/              # Protobuf definitions
    web/                # Frontend assets
    data/               # File storage

------------------------------------------------------------------------

## 📜 License

MIT License
