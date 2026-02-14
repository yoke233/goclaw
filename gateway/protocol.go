package gateway

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// ProtocolVersion 当前协议版本
const ProtocolVersion = "1.0"

// MessageType 消息类型
type MessageType string

const (
	// Request 请求
	MessageTypeRequest MessageType = "request"
	// Response 响应
	MessageTypeResponse MessageType = "response"
	// Notification 通知
	MessageTypeNotification MessageType = "notification"
)

// JSONRPCRequest JSON-RPC 请求
type JSONRPCRequest struct {
	JSONRPC string                 `json:"jsonrpc"`
	ID      string                 `json:"id,omitempty"` // 通知可以没有ID
	Method  string                 `json:"method"`
	Params  map[string]interface{} `json:"params,omitempty"`
}

// UnmarshalJSON allows JSON-RPC id to be either string or number, and normalizes it to a string.
func (r *JSONRPCRequest) UnmarshalJSON(data []byte) error {
	var tmp struct {
		JSONRPC string                 `json:"jsonrpc"`
		ID      interface{}            `json:"id,omitempty"`
		Method  string                 `json:"method"`
		Params  map[string]interface{} `json:"params,omitempty"`
	}
	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}

	r.JSONRPC = tmp.JSONRPC
	r.Method = tmp.Method
	r.Params = tmp.Params

	switch v := tmp.ID.(type) {
	case nil:
		r.ID = ""
	case string:
		r.ID = v
	case float64:
		// JSON numbers are float64; preserve integer-like values as integers.
		if math.Trunc(v) == v {
			r.ID = strconv.FormatInt(int64(v), 10)
		} else {
			r.ID = strconv.FormatFloat(v, 'f', -1, 64)
		}
	default:
		return fmt.Errorf("invalid id type: %T", tmp.ID)
	}

	return nil
}

// JSONRPCResponse JSON-RPC 响应
type JSONRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      string      `json:"id,omitempty"`
	Result  interface{} `json:"result,omitempty"`
	Error   *RPCError   `json:"error,omitempty"`
}

// RPCError RPC 错误
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    string `json:"data,omitempty"`
}

// InvalidParamsError indicates request params are invalid.
type InvalidParamsError struct {
	Message string
}

func (e *InvalidParamsError) Error() string {
	return e.Message
}

// Error codes
const (
	ErrorParseError     = -32700
	ErrorInvalidRequest = -32600
	ErrorMethodNotFound = -32601
	ErrorInvalidParams  = -32602
	ErrorInternalError  = -32603
)

// NewErrorResponse 创建错误响应
func NewErrorResponse(id string, code int, message string) *JSONRPCResponse {
	return &JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &RPCError{
			Code:    code,
			Message: message,
		},
	}
}

// NewSuccessResponse 创建成功响应
func NewSuccessResponse(id string, result interface{}) *JSONRPCResponse {
	return &JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
}

// MethodRegistry 方法注册表
type MethodRegistry struct {
	methods map[string]MethodHandler
}

// MethodNotFoundError is returned when a method is not registered.
type MethodNotFoundError struct {
	Method string
}

func (e *MethodNotFoundError) Error() string {
	return fmt.Sprintf("method not found: %s", e.Method)
}

// MethodHandler 方法处理器
type MethodHandler func(sessionID string, params map[string]interface{}) (interface{}, error)

// NewMethodRegistry 创建方法注册表
func NewMethodRegistry() *MethodRegistry {
	return &MethodRegistry{
		methods: make(map[string]MethodHandler),
	}
}

// Register 注册方法
func (r *MethodRegistry) Register(method string, handler MethodHandler) {
	r.methods[method] = handler
}

// Call 调用方法
func (r *MethodRegistry) Call(method string, sessionID string, params map[string]interface{}) (interface{}, error) {
	handler, ok := r.methods[method]
	if !ok {
		return nil, &MethodNotFoundError{Method: method}
	}
	if handler == nil {
		return nil, fmt.Errorf("nil handler for method: %s", method)
	}
	return handler(sessionID, params)
}

// ParseRequest 解析请求
func ParseRequest(data []byte) (*JSONRPCRequest, error) {
	var req JSONRPCRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, err
	}

	// 验证 JSON-RPC 版本
	if req.JSONRPC != "2.0" {
		return nil, fmt.Errorf("unsupported jsonrpc version: %s", req.JSONRPC)
	}

	// 验证方法名
	if strings.TrimSpace(req.Method) == "" {
		return nil, fmt.Errorf("method is required")
	}

	return &req, nil
}

// EncodeResponse 编码响应
func EncodeResponse(resp *JSONRPCResponse) ([]byte, error) {
	return json.Marshal(resp)
}
