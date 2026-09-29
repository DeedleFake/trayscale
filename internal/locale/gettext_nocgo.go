//go:build !cgo

package locale

func bindTextDomain(domain, dir string) {}

func setLocaleAll(name string) string { return name }

func dGettext(domain, msgid string) string { return msgid }
