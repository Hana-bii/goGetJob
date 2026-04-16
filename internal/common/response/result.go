package response

type Result[T any] struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}

func Success[T any](data T) Result[T] {
	return Result[T]{
		Code:    0,
		Message: "success",
		Data:    data,
	}
}

func Error(code int, message string) Result[any] {
	return Result[any]{
		Code:    code,
		Message: message,
		Data:    nil,
	}
}
