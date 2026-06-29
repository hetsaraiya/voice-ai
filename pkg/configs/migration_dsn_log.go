// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package configs

import "net/url"

// SafeMigrationDSNForLog returns a DSN safe for info-level logs (credentials redacted).
func SafeMigrationDSNForLog(dsn string) string {
	u, err := url.Parse(dsn)
	if err != nil {
		return "<unparseable migration dsn>"
	}
	if u.User != nil {
		u.User = url.UserPassword(u.User.Username(), "REDACTED")
	}
	return u.String()
}