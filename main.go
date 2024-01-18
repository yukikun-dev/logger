package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

func init() {
	rootCmd.PersistentFlags().StringP("port", "p", "9090", "port to run the logging server on")
	rootCmd.PersistentFlags().BoolP("listen", "l", false, "listen on all interfaces")
}

func getLogger() *zap.SugaredLogger {
	encoderCfg := zap.NewProductionEncoderConfig()
	encoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder
	jsonEncoder := zapcore.NewJSONEncoder(encoderCfg)

	encoderCfg = zap.NewDevelopmentEncoderConfig()
	encoderCfg.EncodeLevel = zapcore.CapitalColorLevelEncoder
	encoderCfg.EncodeTime = func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
		enc.AppendString(t.Format("2006-01-02 15:04:05"))
	}
	consoleEncoder := zapcore.NewConsoleEncoder(encoderCfg)

	fileCore := zapcore.NewCore(
		jsonEncoder,
		zapcore.AddSync(&lumberjack.Logger{
			Filename: "logger.log",
		}),
		zap.InfoLevel,
	)

	stdoutCore := zapcore.NewCore(
		consoleEncoder,
		zapcore.AddSync(os.Stdout),
		zap.InfoLevel,
	)

	core := zapcore.NewTee(fileCore, stdoutCore)
	logger := zap.New(core)

	return logger.Sugar()
}
func singleJoiningSlash(a, b string) string {
	aslash := strings.HasSuffix(a, "/")
	bslash := strings.HasPrefix(b, "/")
	switch {
	case aslash && bslash:
		return a + b[1:]
	case !aslash && !bslash:
		return a + "/" + b
	}
	return a + b
}

var rootCmd = &cobra.Command{
	Use:   "logger [target URL]",
	Short: "logger is a simple http proxy server that logs all requests",
	Long:  `logger is a simple http proxy server that logs all requests`,
	PreRunE: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("you must provide a target URL")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		logger := getLogger()

		target, err := url.Parse(args[0])
		if err != nil {
			panic(err)
		}
		logger.Info("proxying to " + target.String())

		proxy := &httputil.ReverseProxy{
			Director: func(req *http.Request) {
				bodyBytes, err := io.ReadAll(req.Body)
				if err != nil {
					logger.Error("Error reading body", zap.Error(err))
					return
				}
				logger.Infow("Received request", "method", req.Method, "url", req.URL.String(), "body", string(bodyBytes))
				req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

				req.URL.Scheme = target.Scheme
				req.URL.Host = target.Host
				req.URL.Path = singleJoiningSlash(target.Path, req.URL.Path)
				req.Host = target.Host
				logger.Info("Forwarding request to ", req.URL.String())
			},
			ModifyResponse: func(res *http.Response) error {
				logger.Infow("Received response", "status", res.Status, "url", res.Request.URL.String(), "method", res.Request.Method)
				return nil
			},
			ErrorHandler: func(writer http.ResponseWriter, request *http.Request, e error) {
				logger.Error("proxy error", zap.Error(e))
			},
			Transport: &http.Transport{
				Proxy: http.ProxyFromEnvironment,
				DialContext: (&net.Dialer{
					Timeout:   30 * time.Second,
					KeepAlive: 30 * time.Second,
					DualStack: true,
				}).DialContext,
				ForceAttemptHTTP2:     true,
				MaxIdleConns:          100,
				IdleConnTimeout:       90 * time.Second,
				TLSHandshakeTimeout:   10 * time.Second,
				ExpectContinueTimeout: 1 * time.Second,
				ResponseHeaderTimeout: 10 * time.Second,
			},
		}

		http.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
			proxy.ServeHTTP(writer, request)
		})

		port := cmd.Flag("port").Value.String()
		if cmd.Flag("listen").Value.String() == "true" {
			logger.Info("listening on http://0.0.0.0:" + port)
			http.ListenAndServe(":"+port, nil)
		} else {
			logger.Info("listening on http://localhost:" + port)
			http.ListenAndServe("localhost:"+port, nil)
		}
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func main() {
	Execute()
}
