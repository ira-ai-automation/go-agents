// Package builtin provides built-in tools for common operations.
package builtin

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ira-ai-automation/go-agents/tools"
)

// FileSystemTool provides file system operations.
type FileSystemTool struct {
	basePath string // Base path for security (optional)
}

// NewFileSystemTool creates a new file system tool.
func NewFileSystemTool(basePath string) *FileSystemTool {
	return &FileSystemTool{
		basePath: basePath,
	}
}

// Name returns the tool name.
func (f *FileSystemTool) Name() string {
	return "filesystem"
}

// Description returns the tool description.
func (f *FileSystemTool) Description() string {
	return "Provides file system operations like reading, writing, listing files and directories"
}

// Schema returns the tool schema.
func (f *FileSystemTool) Schema() *tools.ToolSchema {
	return &tools.ToolSchema{
		Type: "object",
		Properties: map[string]*tools.PropertySpec{
			"operation": {
				Type:        "string",
				Description: "The operation to perform",
				Enum:        []string{"read", "write", "list", "exists", "mkdir", "delete", "stat"},
			},
			"path": {
				Type:        "string",
				Description: "The file or directory path",
			},
			"content": {
				Type:        "string",
				Description: "Content to write (for write operation)",
			},
			"recursive": {
				Type:        "boolean",
				Description: "Whether to perform operation recursively",
				Default:     false,
			},
		},
		Required:    []string{"operation", "path"},
		Description: "Execute file system operations",
	}
}

// Validate checks if the parameters are valid.
func (f *FileSystemTool) Validate(params map[string]interface{}) error {
	operation, ok := params["operation"].(string)
	if !ok || operation == "" {
		return fmt.Errorf("operation parameter is required and must be a string")
	}

	validOps := []string{"read", "write", "list", "exists", "mkdir", "delete", "stat"}
	valid := false
	for _, op := range validOps {
		if operation == op {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("invalid operation: %s. Valid operations: %s", operation, strings.Join(validOps, ", "))
	}

	path, ok := params["path"].(string)
	if !ok || path == "" {
		return fmt.Errorf("path parameter is required and must be a string")
	}

	// Check if write operation has content
	if operation == "write" {
		if _, ok := params["content"]; !ok {
			return fmt.Errorf("content parameter is required for write operation")
		}
	}

	return nil
}

// Execute performs the file system operation.
func (f *FileSystemTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.ToolResult, error) {
	operation := params["operation"].(string)
	path := params["path"].(string)

	// Security check: ensure path is within base path if set
	if f.basePath != "" {
		absPath, err := filepath.Abs(path)
		if err != nil {
			return f.errorResult(fmt.Sprintf("invalid path: %v", err)), nil
		}
		absBasePath, err := filepath.Abs(f.basePath)
		if err != nil {
			return f.errorResult(fmt.Sprintf("invalid base path: %v", err)), nil
		}
		if !strings.HasPrefix(absPath, absBasePath) {
			return f.errorResult("path is outside allowed base path"), nil
		}
		path = absPath
	}

	switch operation {
	case "read":
		return f.readFile(ctx, path)
	case "write":
		content := params["content"].(string)
		return f.writeFile(ctx, path, content)
	case "list":
		recursive := f.getBool(params, "recursive", false)
		return f.listDirectory(ctx, path, recursive)
	case "exists":
		return f.checkExists(ctx, path)
	case "mkdir":
		recursive := f.getBool(params, "recursive", false)
		return f.makeDirectory(ctx, path, recursive)
	case "delete":
		recursive := f.getBool(params, "recursive", false)
		return f.deleteFile(ctx, path, recursive)
	case "stat":
		return f.statFile(ctx, path)
	default:
		return f.errorResult(fmt.Sprintf("unsupported operation: %s", operation)), nil
	}
}

// readFile reads a file and returns its content.
func (f *FileSystemTool) readFile(ctx context.Context, path string) (*tools.ToolResult, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return f.errorResult(fmt.Sprintf("failed to read file: %v", err)), nil
	}

	return &tools.ToolResult{
		Success: true,
		Data: map[string]interface{}{
			"content": string(content),
			"size":    len(content),
			"path":    path,
		},
	}, nil
}

// writeFile writes content to a file.
func (f *FileSystemTool) writeFile(ctx context.Context, path, content string) (*tools.ToolResult, error) {
	// Create directory if it doesn't exist
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return f.errorResult(fmt.Sprintf("failed to create directory: %v", err)), nil
	}

	err := os.WriteFile(path, []byte(content), 0644)
	if err != nil {
		return f.errorResult(fmt.Sprintf("failed to write file: %v", err)), nil
	}

	return &tools.ToolResult{
		Success: true,
		Data: map[string]interface{}{
			"path":       path,
			"size":       len(content),
			"written_at": time.Now(),
		},
	}, nil
}

// listDirectory lists files and directories.
func (f *FileSystemTool) listDirectory(ctx context.Context, path string, recursive bool) (*tools.ToolResult, error) {
	var files []map[string]interface{}

	if recursive {
		err := filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			relPath, _ := filepath.Rel(path, filePath)
			files = append(files, map[string]interface{}{
				"name":          info.Name(),
				"path":          filePath,
				"relative_path": relPath,
				"is_dir":        info.IsDir(),
				"size":          info.Size(),
				"modified":      info.ModTime(),
				"permissions":   info.Mode().String(),
			})
			return nil
		})

		if err != nil {
			return f.errorResult(fmt.Sprintf("failed to walk directory: %v", err)), nil
		}
	} else {
		entries, err := os.ReadDir(path)
		if err != nil {
			return f.errorResult(fmt.Sprintf("failed to read directory: %v", err)), nil
		}

		for _, entry := range entries {
			info, err := entry.Info()
			if err != nil {
				continue
			}

			files = append(files, map[string]interface{}{
				"name":        entry.Name(),
				"path":        filepath.Join(path, entry.Name()),
				"is_dir":      entry.IsDir(),
				"size":        info.Size(),
				"modified":    info.ModTime(),
				"permissions": info.Mode().String(),
			})
		}
	}

	return &tools.ToolResult{
		Success: true,
		Data: map[string]interface{}{
			"files": files,
			"count": len(files),
			"path":  path,
		},
	}, nil
}

// checkExists checks if a file or directory exists.
func (f *FileSystemTool) checkExists(ctx context.Context, path string) (*tools.ToolResult, error) {
	_, err := os.Stat(path)
	exists := !os.IsNotExist(err)

	data := map[string]interface{}{
		"exists": exists,
		"path":   path,
	}

	if exists {
		info, _ := os.Stat(path)
		data["is_dir"] = info.IsDir()
		data["size"] = info.Size()
		data["modified"] = info.ModTime()
	}

	return &tools.ToolResult{
		Success: true,
		Data:    data,
	}, nil
}

// makeDirectory creates a directory.
func (f *FileSystemTool) makeDirectory(ctx context.Context, path string, recursive bool) (*tools.ToolResult, error) {
	var err error
	if recursive {
		err = os.MkdirAll(path, 0755)
	} else {
		err = os.Mkdir(path, 0755)
	}

	if err != nil {
		return f.errorResult(fmt.Sprintf("failed to create directory: %v", err)), nil
	}

	return &tools.ToolResult{
		Success: true,
		Data: map[string]interface{}{
			"path":       path,
			"created_at": time.Now(),
			"recursive":  recursive,
		},
	}, nil
}

// deleteFile deletes a file or directory.
func (f *FileSystemTool) deleteFile(ctx context.Context, path string, recursive bool) (*tools.ToolResult, error) {
	var err error
	if recursive {
		err = os.RemoveAll(path)
	} else {
		err = os.Remove(path)
	}

	if err != nil {
		return f.errorResult(fmt.Sprintf("failed to delete: %v", err)), nil
	}

	return &tools.ToolResult{
		Success: true,
		Data: map[string]interface{}{
			"path":       path,
			"deleted_at": time.Now(),
			"recursive":  recursive,
		},
	}, nil
}

// statFile gets file information.
func (f *FileSystemTool) statFile(ctx context.Context, path string) (*tools.ToolResult, error) {
	info, err := os.Stat(path)
	if err != nil {
		return f.errorResult(fmt.Sprintf("failed to stat file: %v", err)), nil
	}

	return &tools.ToolResult{
		Success: true,
		Data: map[string]interface{}{
			"name":        info.Name(),
			"path":        path,
			"size":        info.Size(),
			"is_dir":      info.IsDir(),
			"modified":    info.ModTime(),
			"permissions": info.Mode().String(),
		},
	}, nil
}

// getBool safely gets a boolean parameter with a default value.
func (f *FileSystemTool) getBool(params map[string]interface{}, key string, defaultValue bool) bool {
	if val, ok := params[key]; ok {
		if b, ok := val.(bool); ok {
			return b
		}
	}
	return defaultValue
}

// errorResult creates an error result.
func (f *FileSystemTool) errorResult(message string) *tools.ToolResult {
	return &tools.ToolResult{
		Success: false,
		Error:   message,
	}
}
