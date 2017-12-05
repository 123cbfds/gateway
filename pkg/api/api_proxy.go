package api

import (
	"net/http"

	"github.com/fagongzi/gateway/pkg/model"
	"github.com/labstack/echo"
)

// LogLevel loglevel model
type LogLevel struct {
	Addr  string `json:"addr"`
	Level string `json:"level"`
}

func (s *Server) initAPIOfProxies() {
	s.api.GET("/api/v1/proxies", s.listProxies())
	s.api.PUT("/api/v1/proxies", s.updateLogLevel())
}

func (s *Server) listProxies() echo.HandlerFunc {
	return func(c echo.Context) error {
		var errstr string
		code := CodeSuccess

		registor, _ := s.store.(model.Register)

		proxies, err := registor.GetProxies()
		if err != nil {
			errstr = err.Error()
			code = CodeError
		}

		return c.JSON(http.StatusOK, &Result{
			Code:  code,
			Error: errstr,
			Value: proxies,
		})
	}
}

func (s *Server) updateLogLevel() echo.HandlerFunc {
	return func(c echo.Context) error {
		var errstr string
		code := CodeSuccess

		level := &LogLevel{}
		err := readJSONFromReader(level, c.Request().Body)

		if nil != err {
			errstr = err.Error()
			code = CodeError
		} else {
			registor, _ := s.store.(model.Register)

			err := registor.ChangeLogLevel(level.Addr, level.Level)
			if err != nil {
				errstr = err.Error()
				code = CodeError
			}
		}

		return c.JSON(http.StatusOK, &Result{
			Code:  code,
			Error: errstr,
		})
	}
}
