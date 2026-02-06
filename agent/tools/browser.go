package tools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/smallnest/dogclaw/goclaw/internal/logger"
	"go.uber.org/zap"
)

// BrowserTool 浏览器工具
type BrowserTool struct {
	headless bool
	timeout  time.Duration
	tempDir  string
}

// NewBrowserTool 创建浏览器工具
func NewBrowserTool(headless bool, timeout int) *BrowserTool {
	var t time.Duration
	if timeout > 0 {
		t = time.Duration(timeout) * time.Second
	} else {
		t = 30 * time.Second
	}

	// 创建临时目录用于保存截图
	tempDir, err := os.MkdirTemp("", "goclaw-browser-")
	if err != nil {
		logger.Warn("Failed to create temp dir for browser", zap.Error(err))
		tempDir = os.TempDir()
	}

	return &BrowserTool{
		headless: headless,
		timeout:  t,
		tempDir:  tempDir,
	}
}

// Close 关闭浏览器工具，清理资源
func (b *BrowserTool) Close() error {
	if b.tempDir != "" && b.tempDir != os.TempDir() {
		if err := os.RemoveAll(b.tempDir); err != nil {
			logger.Warn("Failed to remove temp dir", zap.Error(err))
		}
	}
	return nil
}

// BrowserNavigate 浏览器导航到指定 URL
func (b *BrowserTool) BrowserNavigate(ctx context.Context, params map[string]interface{}) (string, error) {
	urlStr, ok := params["url"].(string)
	if !ok {
		return "", fmt.Errorf("url parameter is required")
	}

	// 验证 URL
	if _, err := url.Parse(urlStr); err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}

	logger.Info("Browser navigating to", zap.String("url", urlStr))

	// 创建带有超时的上下文
	ctx, cancel := context.WithTimeout(ctx, b.timeout)
	defer cancel()

	// 创建 chromedp 上下文
	allocCtx, cancel := chromedp.NewContext(ctx)
	defer cancel()

	// 设置超时
	allocCtx, cancel = context.WithTimeout(allocCtx, b.timeout)
	defer cancel()

	var currentURL, title string
	var bodyText string

	// 执行导航操作
	err := chromedp.Run(allocCtx,
		chromedp.Navigate(urlStr),
		chromedp.WaitReady("body", chromedp.ByQuery),
		chromedp.Sleep(500*time.Millisecond),
		chromedp.Location(&currentURL),
		chromedp.Title(&title),
		chromedp.OuterHTML("body", &bodyText, chromedp.ByQuery),
	)

	if err != nil {
		return "", fmt.Errorf("failed to navigate: %w", err)
	}

	return fmt.Sprintf("Navigated to: %s\nTitle: %s\nPage size: %d bytes", currentURL, title, len(bodyText)), nil
}

// BrowserScreenshot 截取页面截图
func (b *BrowserTool) BrowserScreenshot(ctx context.Context, params map[string]interface{}) (string, error) {
	var urlStr string
	var width, height int

	// 获取参数
	if u, ok := params["url"].(string); ok {
		urlStr = u
	}
	if w, ok := params["width"].(float64); ok {
		width = int(w)
	} else {
		width = 1920
	}
	if h, ok := params["height"].(float64); ok {
		height = int(h)
	} else {
		height = 1080
	}

	logger.Info("Browser screenshot", zap.String("url", urlStr), zap.Int("width", width), zap.Int("height", height))

	// 创建带有超时的上下文
	ctx, cancel := context.WithTimeout(context.Background(), b.timeout)
	defer cancel()

	// 创建 chromedp 上下文
	allocCtx, cancel := chromedp.NewContext(ctx)
	defer cancel()

	// 创建任务
	tasks := chromedp.Tasks{}

	// 设置视口大小
	tasks = append(tasks, chromedp.EmulateViewport(int64(width), int64(height)))

	// 如果指定了 URL，先导航
	if urlStr != "" {
		tasks = append(tasks,
			chromedp.Navigate(urlStr),
			chromedp.WaitReady("body", chromedp.ByQuery),
			chromedp.Sleep(500*time.Millisecond),
		)
	}

	var title, currentURL string
	var screenshot []byte

	// 添加截图和获取信息任务
	tasks = append(tasks,
		chromedp.Location(&currentURL),
		chromedp.Title(&title),
		chromedp.FullScreenshot(&screenshot, 100),
	)

	// 执行任务
	err := chromedp.Run(allocCtx, tasks)
	if err != nil {
		return "", fmt.Errorf("failed to capture screenshot: %w", err)
	}

	// 保存到临时文件
	filename := fmt.Sprintf("screenshot_%d.png", time.Now().Unix())
	filepath := b.tempDir + string(os.PathSeparator) + filename
	if err := os.WriteFile(filepath, screenshot, 0644); err != nil {
		return "", fmt.Errorf("failed to save screenshot: %w", err)
	}

	// 转换为 Base64 用于传输
	base64Str := base64.StdEncoding.EncodeToString(screenshot)

	return fmt.Sprintf("Screenshot saved to: %s\nTitle: %s\nURL: %s\nBase64 length: %d bytes\nImage URL: file://%s",
		filepath, title, currentURL, len(base64Str), filepath), nil
}

// BrowserExecuteScript 在浏览器中执行 JavaScript
func (b *BrowserTool) BrowserExecuteScript(ctx context.Context, params map[string]interface{}) (string, error) {
	script, ok := params["script"].(string)
	if !ok {
		return "", fmt.Errorf("script parameter is required")
	}

	urlStr := ""
	if u, ok := params["url"].(string); ok {
		urlStr = u
	}

	logger.Info("Browser executing script", zap.String("url", urlStr), zap.String("script", script))

	// 创建带有超时的上下文
	ctx, cancel := context.WithTimeout(ctx, b.timeout)
	defer cancel()

	// 创建 chromedp 上下文
	allocCtx, cancel := chromedp.NewContext(ctx)
	defer cancel()

	// 创建任务
	tasks := chromedp.Tasks{}

	// 如果指定了 URL，先导航
	if urlStr != "" {
		tasks = append(tasks,
			chromedp.Navigate(urlStr),
			chromedp.WaitReady("body", chromedp.ByQuery),
			chromedp.Sleep(500*time.Millisecond),
		)
	}

	var result interface{}

	// 添加执行脚本任务
	tasks = append(tasks, chromedp.Evaluate(script, &result))

	// 执行任务
	err := chromedp.Run(allocCtx, tasks)
	if err != nil {
		return "", fmt.Errorf("failed to execute script: %w", err)
	}

	// 格式化结果
	resultJSON, err := formatResult(result)
	if err != nil {
		return "", fmt.Errorf("failed to format result: %w", err)
	}

	return resultJSON, nil
}

// BrowserClick 点击页面元素
func (b *BrowserTool) BrowserClick(ctx context.Context, params map[string]interface{}) (string, error) {
	urlStr := ""
	selector, ok := params["selector"].(string)
	if !ok {
		return "", fmt.Errorf("selector parameter is required")
	}

	if u, ok := params["url"].(string); ok {
		urlStr = u
	}

	logger.Info("Browser clicking element", zap.String("url", urlStr), zap.String("selector", selector))

	// 创建带有超时的上下文
	ctx, cancel := context.WithTimeout(context.Background(), b.timeout)
	defer cancel()

	// 创建 chromedp 上下文
	allocCtx, cancel := chromedp.NewContext(ctx)
	defer cancel()

	// 创建任务
	tasks := chromedp.Tasks{}

	// 如果指定了 URL，先导航
	if urlStr != "" {
		tasks = append(tasks,
			chromedp.Navigate(urlStr),
			chromedp.WaitReady("body", chromedp.ByQuery),
			chromedp.Sleep(500*time.Millisecond),
		)
	}

	// 添加点击任务
	tasks = append(tasks,
		chromedp.WaitVisible(selector, chromedp.ByQuery),
		chromedp.Click(selector, chromedp.ByQuery),
		chromedp.Sleep(500*time.Millisecond),
	)

	// 执行任务
	err := chromedp.Run(allocCtx, tasks)
	if err != nil {
		return "", fmt.Errorf("failed to click element: %w", err)
	}

	return fmt.Sprintf("Successfully clicked element: %s", selector), nil
}

// BrowserFillInput 填写输入框
func (b *BrowserTool) BrowserFillInput(ctx context.Context, params map[string]interface{}) (string, error) {
	urlStr := ""
	selector, ok := params["selector"].(string)
	if !ok {
		return "", fmt.Errorf("selector parameter is required")
	}

	value, ok := params["value"].(string)
	if !ok {
		return "", fmt.Errorf("value parameter is required")
	}

	if u, ok := params["url"].(string); ok {
		urlStr = u
	}

	logger.Info("Browser filling input", zap.String("url", urlStr), zap.String("selector", selector), zap.String("value", "***"))

	// 创建带有超时的上下文
	ctx, cancel := context.WithTimeout(context.Background(), b.timeout)
	defer cancel()

	// 创建 chromedp 上下文
	allocCtx, cancel := chromedp.NewContext(ctx)
	defer cancel()

	// 创建任务
	tasks := chromedp.Tasks{}

	// 如果指定了 URL，先导航
	if urlStr != "" {
		tasks = append(tasks,
			chromedp.Navigate(urlStr),
			chromedp.WaitReady("body", chromedp.ByQuery),
			chromedp.Sleep(500*time.Millisecond),
		)
	}

	// 添加填写任务
	tasks = append(tasks,
		chromedp.WaitVisible(selector, chromedp.ByQuery),
		chromedp.Focus(selector, chromedp.ByQuery),
		chromedp.SendKeys(selector, value, chromedp.ByQuery),
		chromedp.Sleep(500*time.Millisecond),
	)

	// 执行任务
	err := chromedp.Run(allocCtx, tasks)
	if err != nil {
		return "", fmt.Errorf("failed to input value: %w", err)
	}

	return fmt.Sprintf("Successfully filled input: %s", selector), nil
}

// BrowserGetText 获取页面文本内容
func (b *BrowserTool) BrowserGetText(ctx context.Context, params map[string]interface{}) (string, error) {
	urlStr, ok := params["url"].(string)
	if !ok {
		return "", fmt.Errorf("url parameter is required")
	}

	logger.Info("Browser getting text", zap.String("url", urlStr))

	// 创建带有超时的上下文
	ctx, cancel := context.WithTimeout(context.Background(), b.timeout)
	defer cancel()

	// 创建 chromedp 上下文
	allocCtx, cancel := chromedp.NewContext(ctx)
	defer cancel()

	var bodyText string

	// 执行获取文本操作
	err := chromedp.Run(allocCtx,
		chromedp.Navigate(urlStr),
		chromedp.WaitReady("body", chromedp.ByQuery),
		chromedp.Sleep(500*time.Millisecond),
		chromedp.OuterHTML("body", &bodyText, chromedp.ByQuery),
	)

	if err != nil {
		return "", fmt.Errorf("failed to get page text: %w", err)
	}

	// 简化文本（移除多余空行）
	text := htmlToText(bodyText)

	// 限制长度
	if len(text) > 10000 {
		text = text[:10000] + "\n\n... (truncated)"
	}

	return fmt.Sprintf("Page text from %s:\n\n%s", urlStr, text), nil
}

// GetTools 获取所有浏览器工具
func (b *BrowserTool) GetTools() []Tool {
	return []Tool{
		NewBaseTool(
			"browser_navigate",
			"Navigate browser to a URL and wait for it to load",
			map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"url": map[string]interface{}{
						"type":        "string",
						"description": "URL to navigate to (must start with http:// or https://)",
					},
				},
				"required": []string{"url"},
			},
			b.BrowserNavigate,
		),
		NewBaseTool(
			"browser_screenshot",
			"Take a screenshot of the current page or navigate to a URL first",
			map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"url": map[string]interface{}{
						"type":        "string",
						"description": "URL to navigate to before screenshot (optional)",
					},
					"width": map[string]interface{}{
						"type":        "number",
						"description": "Screenshot width in pixels (default: 1920)",
					},
					"height": map[string]interface{}{
						"type":        "number",
						"description": "Screenshot height in pixels (default: 1080)",
					},
				},
			},
			b.BrowserScreenshot,
		),
		NewBaseTool(
			"browser_execute_script",
			"Execute JavaScript code in the browser console",
			map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"script": map[string]interface{}{
						"type":        "string",
						"description": "JavaScript code to execute",
					},
					"url": map[string]interface{}{
						"type":        "string",
						"description": "URL to navigate to before executing (optional)",
					},
				},
			},
			b.BrowserExecuteScript,
		),
		NewBaseTool(
			"browser_click",
			"Click an element on the page using CSS selector",
			map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"selector": map[string]interface{}{
						"type":        "string",
						"description": "CSS selector of the element to click (e.g., '#button', '.submit', '[name=\"submit\"]')",
					},
					"url": map[string]interface{}{
						"type":        "string",
						"description": "URL to navigate to before clicking (optional)",
					},
				},
				"required": []string{"selector"},
			},
			b.BrowserClick,
		),
		NewBaseTool(
			"browser_fill_input",
			"Fill an input field with text",
			map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"selector": map[string]interface{}{
						"type":        "string",
						"description": "CSS selector of the input field (e.g., '#username', 'input[name=\"search\"]')",
					},
					"value": map[string]interface{}{
						"type":        "string",
						"description": "Text to fill into the input field",
					},
					"url": map[string]interface{}{
						"type":        "string",
						"description": "URL to navigate to before filling (optional)",
					},
				},
				"required": []string{"selector", "value"},
			},
			b.BrowserFillInput,
		),
		NewBaseTool(
			"browser_get_text",
			"Get the text content of a web page",
			map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"url": map[string]interface{}{
						"type":        "string",
						"description": "URL of the page to get text from",
					},
				},
				"required": []string{"url"},
			},
			b.BrowserGetText,
		),
	}
}

// htmlToText 将 HTML 转换为纯文本
func htmlToText(html string) string {
	// 简化实现：移除 HTML 标签
	text := ""
	inTag := false
	for i := 0; i < len(html); i++ {
		if html[i] == '<' {
			inTag = true
			continue
		}
		if html[i] == '>' {
			inTag = false
			continue
		}
		if !inTag {
			text += string(html[i])
		}
	}
	return text
}

// formatResult 格式化执行结果
func formatResult(result interface{}) (string, error) {
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}
