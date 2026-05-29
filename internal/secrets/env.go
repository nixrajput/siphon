package secrets

import (
	"errors"
	"os"
	"strings"

	"github.com/nixrajput/siphon/internal/errs"
)

// Env resolves "env:VAR" refs to os.Getenv("VAR").
type Env struct{}

func (Env) Scheme() string { return "env" }

func (Env) Resolve(ref string) (string, error) {
	name := strings.TrimPrefix(ref, "env:")
	if name == "" {
		return "", &errs.Error{
			Op:    "secrets.env.resolve",
			Code:  errs.CodeUser,
			Cause: errors.New("env: ref missing variable name"),
			Hint:  "use env:VARIABLE_NAME",
		}
	}
	val, ok := os.LookupEnv(name)
	if !ok {
		return "", &errs.Error{
			Op:    "secrets.env.resolve",
			Code:  errs.CodeUser,
			Cause: errs.ErrSecretUnresolved,
			Hint:  "set environment variable " + name,
		}
	}
	return val, nil
}
