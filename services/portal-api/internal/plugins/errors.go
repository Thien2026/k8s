package plugins

import "errors"

var (
	ErrNotFound          = errors.New("plugin không tồn tại")
	ErrCannotDisableCore = errors.New("không thể tắt plugin core")
)
