// Package mask esconde dados pessoais nos logs (LGPD): CPF/CNPJ e e-mail nunca aparecem inteiros.
package mask

import "strings"

// Document mantém só os dois últimos dígitos: 52998224725 → ***.***.***-25.
func Document(doc string) string {
	digits := make([]rune, 0, len(doc))
	for _, r := range doc {
		if r >= '0' && r <= '9' {
			digits = append(digits, r)
		}
	}
	if len(digits) < 2 {
		return "***"
	}
	tail := string(digits[len(digits)-2:])
	if len(digits) > 11 {
		return "**.***.***/****-" + tail
	}
	return "***.***.***-" + tail
}

// Email mantém a primeira letra e o domínio: ana.souza@example.com → a***@example.com.
func Email(email string) string {
	at := strings.LastIndex(email, "@")
	if at < 1 {
		return "***"
	}
	return email[:1] + "***" + email[at:]
}
