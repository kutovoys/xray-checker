---
title: Development Guide
description: Development guide
---

### Setting Up Development Environment

1. Requirements:

   - Go 1.27.3 or later
   - Git
   - Make (optional, for using Makefile)

2. Clone the repository:

```bash
git clone https://github.com/kutovoys/xray-checker.git
cd xray-checker
```

3. Install dependencies:

```bash
go mod download
```

4. Build the project:

```bash
make build
# or
go build -o xray-checker
```

### Project Structure

```
.
├── checker/       # Proxy checking logic
├── config/        # Configuration handling
├── geo/           # Geo files (geoip.dat, geosite.dat)
├── logger/        # Structured logging
├── metrics/       # Prometheus metrics
├── models/        # Data models
├── subscription/  # Subscription parsing and management
├── web/           # Web interface, API, and assets
├── xray/          # Xray integration and runner
├── go.mod         # Go modules file
└── main.go        # Application entry point
```

### Making Changes

1. Create a new branch:

```bash
git checkout -b feature/your-feature-name
```

2. Make your changes
3. Run tests
4. Update documentation if needed
5. Submit a pull request

### Local Testing

1. Set up test configuration:

```bash
export SUBSCRIPTION_URL=your_test_subscription
```

2. Run in development mode:

```bash
go run main.go
```

3. Run with specific features:

```bash
go run main.go --proxy-check-method=status --metrics-protected=true
```
