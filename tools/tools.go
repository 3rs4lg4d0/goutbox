//go:build tools

package tools

import (
	_ "github.com/golang-migrate/migrate/v4/cmd/migrate"
	_ "github.com/vektra/mockery/v2/cmd"
)
