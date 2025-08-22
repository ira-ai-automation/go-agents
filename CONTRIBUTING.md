# Contributing to Agentarium Core

We welcome contributions to Agentarium Core! This document provides guidelines for contributing to the project.

## Table of Contents

- [Getting Started](#getting-started)
- [Development Setup](#development-setup)
- [Contributing Guidelines](#contributing-guidelines)
- [Code Standards](#code-standards)
- [Submitting Changes](#submitting-changes)
- [Reporting Issues](#reporting-issues)
- [Community](#community)

## Getting Started

### Prerequisites

- Go 1.21 or later
- Git
- Basic understanding of Go and concurrent programming

### Fork and Clone

1. Fork the repository on GitHub
2. Clone your fork locally:

```bash
git clone https://github.com/YOUR_USERNAME/agentarium-core.git
cd agentarium-core
```

3. Add the original repository as upstream:

```bash
git remote add upstream https://github.com/ira-ai-automation/agentarium-core.git
```

## Development Setup

### Install Dependencies

```bash
go mod download
go mod tidy
```

### Build and Test

```bash
# Build all packages
go build ./...

# Run tests
go test ./...

# Run examples
cd examples/simple_agent && go run main.go
cd ../memory_agents && go run main.go
```

### Environment Setup

For testing LLM features, set up environment variables:

```bash
# Optional: for real LLM testing
export OPENAI_API_KEY="your-key-here"
export ANTHROPIC_API_KEY="your-key-here"

# The examples work with mock providers if no keys are set
```

## Contributing Guidelines

### Types of Contributions

We welcome various types of contributions:

- **Bug fixes**: Fix issues in existing code
- **New features**: Add new functionality
- **Documentation**: Improve docs, examples, and comments
- **Performance**: Optimize existing code
- **Tests**: Add or improve test coverage
- **Examples**: Create new example applications

### Before You Start

1. **Check existing issues**: Look for related issues or discussions
2. **Create an issue**: For significant changes, create an issue first to discuss
3. **Small changes**: For small fixes, you can directly create a PR

## Code Standards

### Go Guidelines

- Follow [Effective Go](https://golang.org/doc/effective_go.html)
- Use `gofmt` for formatting
- Use `go vet` to check for common mistakes
- Write tests for new functionality
- Add comments for exported functions and types

### Code Organization

```
agentarium-core/
├── agent/          # Core agent system
├── llm/            # LLM integration
├── memory/         # Memory and vector systems
├── examples/       # Working examples
└── docs/           # Additional documentation
```

### Naming Conventions

- **Packages**: Short, lowercase, single word
- **Files**: Lowercase with underscores (e.g., `memory_agent.go`)
- **Types**: PascalCase (e.g., `MemoryAgent`)
- **Functions**: PascalCase for exported, camelCase for internal
- **Constants**: PascalCase or SCREAMING_SNAKE_CASE

### Documentation

- Add package-level documentation
- Document all exported types and functions
- Include examples in documentation
- Update README.md for significant changes

### Testing

- Write unit tests for new functionality
- Use table-driven tests where appropriate
- Test error conditions
- Aim for good test coverage (>80%)

Example test structure:

```go
func TestAgentCreation(t *testing.T) {
    tests := []struct {
        name    string
        config  Config
        wantErr bool
    }{
        {
            name:    "valid config",
            config:  validConfig,
            wantErr: false,
        },
        {
            name:    "invalid config",
            config:  invalidConfig,
            wantErr: true,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            agent, err := NewAgent(tt.config)
            if (err != nil) != tt.wantErr {
                t.Errorf("NewAgent() error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            if !tt.wantErr && agent == nil {
                t.Error("Expected agent to be created")
            }
        })
    }
}
```

## Submitting Changes

### Commit Messages

Use conventional commit format:

```
type(scope): description

[optional body]

[optional footer]
```

Types:
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation changes
- `style`: Code style changes
- `refactor`: Code refactoring
- `test`: Adding tests
- `chore`: Maintenance tasks

Examples:
```
feat(memory): add vector similarity search
fix(agent): resolve race condition in message handling
docs(readme): update installation instructions
```

### Pull Request Process

1. **Create a branch**: Use a descriptive name
   ```bash
   git checkout -b feature/vector-search-optimization
   ```

2. **Make changes**: Follow the code standards above

3. **Test thoroughly**: Ensure all tests pass
   ```bash
   go test ./...
   go build ./...
   ```

4. **Commit changes**: Use conventional commit messages
   ```bash
   git add .
   git commit -m "feat(memory): add advanced vector similarity search"
   ```

5. **Push to your fork**:
   ```bash
   git push origin feature/vector-search-optimization
   ```

6. **Create Pull Request**: 
   - Use the GitHub interface
   - Fill out the PR template
   - Link related issues
   - Request review

### Pull Request Template

When creating a PR, include:

- **Description**: What does this PR do?
- **Type**: Feature, bugfix, documentation, etc.
- **Testing**: How was this tested?
- **Breaking Changes**: Any breaking changes?
- **Related Issues**: Links to related issues

## Reporting Issues

### Bug Reports

Include:
- Go version (`go version`)
- Operating system
- Steps to reproduce
- Expected vs actual behavior
- Relevant code snippets
- Error messages/logs

### Feature Requests

Include:
- Use case description
- Proposed solution
- Alternatives considered
- Additional context

### Performance Issues

Include:
- Benchmark results
- Profiling data if available
- System specifications
- Workload characteristics

## Community

### Communication

- **GitHub Issues**: Bug reports and feature requests
- **GitHub Discussions**: General questions and discussions
- **Code Reviews**: Constructive feedback on PRs

### Code of Conduct

- Be respectful and inclusive
- Focus on constructive feedback
- Help newcomers learn
- Maintain a professional tone

### Recognition

Contributors are recognized through:
- GitHub contributor graphs
- Release notes acknowledgments
- Community highlights

## Development Tips

### Local Development

```bash
# Run specific example
cd examples/memory_agents
go run main.go

# Test specific package
go test ./memory -v

# Build with race detection
go build -race ./...

# Format code
go fmt ./...

# Lint code (if golangci-lint is installed)
golangci-lint run
```

### Debugging

- Use `fmt.Printf` for simple debugging
- Use `go run -race` to detect race conditions
- Use `go test -v` for verbose test output
- Use delve debugger for complex issues

### Common Patterns

```go
// Error handling
if err != nil {
    return fmt.Errorf("operation failed: %w", err)
}

// Context usage
func (a *Agent) ProcessMessage(ctx context.Context, msg Message) error {
    select {
    case <-ctx.Done():
        return ctx.Err()
    default:
        // Process message
    }
}

// Goroutine cleanup
func (a *Agent) Start(ctx context.Context) error {
    a.wg.Add(1)
    go func() {
        defer a.wg.Done()
        // Agent work
    }()
}
```

## Questions?

If you have questions about contributing:

1. Check existing documentation
2. Search GitHub issues
3. Create a new discussion
4. Reach out to maintainers

Thank you for contributing to Agentarium Core! 🚀
