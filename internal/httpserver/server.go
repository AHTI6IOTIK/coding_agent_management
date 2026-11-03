package httpserver

import (
	_ "embed"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/pprof"
	"strings"
	"unicode"

	"github.com/go-playground/validator/v10"
	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

//go:embed calculator.html
var calculatorPage string

type customValidator struct {
	validator *validator.Validate
}

func (v customValidator) Validate(value any) error {
	return v.validator.Struct(value)
}

type echoRequest struct {
	Message string  `json:"message" validate:"required,max=1024"`
	Calc    *string `json:"calc"`
}

type echoResponse struct {
	Message string  `json:"message"`
	Result  *string `json:"result,omitempty"`
}

func NewApp(version string) *echo.Echo {
	app := echo.New()
	app.Validator = customValidator{validator: validator.New(validator.WithRequiredStructEnabled())}
	slog.Info("application version", "version", version)

	app.GET("/", func(context echo.Context) error {
		return context.JSON(http.StatusOK, map[string]string{"message": "hello world"})
	})
	app.GET("/calculator", calculatorHandler)
	app.GET("/health", func(context echo.Context) error {
		return context.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})
	app.GET("/version", versionHandler(version))
	app.POST("/echo", echoMessage)

	return app
}

// calculatorHandler handles GET /calculator and returns the calculator page.
func calculatorHandler(context echo.Context) error {
	return context.HTML(http.StatusOK, calculatorPage)
}

// versionHandler handles GET /version and returns the application version.
func versionHandler(version string) echo.HandlerFunc {
	return func(context echo.Context) error {
		return context.JSON(http.StatusOK, map[string]string{"version": version})
	}
}

func MetricsHandler() http.Handler {
	return promhttp.Handler()
}

func PprofHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	return mux
}

func echoMessage(context echo.Context) error {
	request := new(echoRequest)
	if err := context.Bind(request); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}
	if err := context.Validate(request); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}

	response := echoResponse{Message: request.Message}
	if request.Calc != nil {
		result := calculate(*request.Calc)
		response.Result = &result
	}

	return context.JSON(http.StatusOK, response)
}

func calculate(expression string) string {
	expression = strings.Map(func(character rune) rune {
		if unicode.IsSpace(character) {
			return -1
		}
		return character
	}, expression)
	if expression == "" {
		return "NaN"
	}

	sum := new(big.Int)
	for _, operand := range strings.Split(expression, "+") {
		value, ok := new(big.Int).SetString(operand, 10)
		if !ok || value.Sign() < 0 {
			return "NaN"
		}
		sum.Add(sum, value)
	}

	return sum.String()
}
