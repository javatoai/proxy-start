package application

import "errors"

var (
	ErrNotFound           = errors.New("找不到该启动项或代理配置")
	ErrCredentialNotFound = errors.New("代理凭据不存在")
	ErrAlreadyRunning     = errors.New("程序已在运行；请使用重启来重新应用代理设置")
	ErrClosed             = errors.New("客户端正在退出")
	ErrUnsupportedBypass  = errors.New("当前强制代理模式不支持按域名绕过")
)
