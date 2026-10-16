package commandactivation

// ValidateRule expõe a validação estrutural para projetores compostos. Ela
// não verifica decisão/grant: a autoridade dessas relações pertence ao
// protocolo commandautomation.
func ValidateRule(rule Rule) error { return validateRule(rule) }

// ValidateClaim expõe a validação estrutural para o diff agregado sem alterar
// o formato persistido de Claim.
func ValidateClaim(claim Claim) error { return validateClaim(claim) }
