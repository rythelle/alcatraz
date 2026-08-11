package proxy

import (
	"regexp"
)

type SensitivePattern struct {
	Name        string
	Regex       *regexp.Regexp
	Replacement string
}

// patternValidators attaches a structural validator to a pattern by name.
// A pattern listed here is redacted only when its validator confirms the match
// (correct check digits / checksum); this lets checksum-backed patterns match
// aggressively (bare digits, no surrounding context) without flooding false
// positives on fake data — the kind that fills test fixtures and docs. Keying
// by name keeps the pattern table itself as compact positional literals.
var patternValidators = map[string]func(match string) bool{
	"cpf_formatted":     validateCPF,
	"cpf_context":       validateCPF,
	"cpf_bare":          validateCPF,
	"cnpj_formatted":    validateCNPJ,
	"cnpj_context":      validateCNPJ,
	"cnpj_bare":         validateCNPJ,
	"pis_nis":           validatePIS,
	"cns_sus":           validateCNS,
	"iban":              validateIBAN,
	"credit_card":       validateCard,
	"payment_card":      validateCard,
	"payment_card_bare": validateCard,
	"sin_context":       validateSIN,
	"imei_context":      validateIMEI,
	"bsn_context":       validateBSN,
	"nif_pt_context":    validateNIF,
	"dni_es_context":    validateDNI,
	"aadhaar_context":   validateAadhaar,
}

// StrictPatterns are enabled only under `mode: strict` in the Guard rules
// file. They are context-free variants of tier-2 patterns whose formats are
// distinctive enough to be worth matching bare, at the cost of more false
// positives. They carry no checksum validator.
var StrictPatterns = []SensitivePattern{
	{"ssn_bare", re(`\b\d{3}-\d{2}-\d{4}\b`), "[REDACTED_BY_ALCATRAZ_SSN]"},
	{"mercosul_plate", re(`\b[A-Z]{3}\d[A-Z]\d{2}\b`), "[REDACTED_BY_ALCATRAZ_PLATE]"},
	{"cep_hyphenated", re(`\b\d{5}-\d{3}\b`), "[REDACTED_BY_ALCATRAZ_CEP]"},
}

var SensitivePatterns = []SensitivePattern{
	// ═══════════════════════════════════════════════════════════════════════
	// 1. API KEYS & TOKENS
	// ═══════════════════════════════════════════════════════════════════════
	{"openai_key", re(`\bsk-[a-zA-Z0-9]{48,}\b`), "[REDACTED_BY_ALCATRAZ_OPENAI_KEY]"},
	{"openai_project_key", re(`\bsk-proj-[a-zA-Z0-9\-_]{60,}\b`), "[REDACTED_BY_ALCATRAZ_OPENAI_PROJ]"},
	{"anthropic_key", re(`\bsk-ant-[a-zA-Z0-9_-]{20,}\b`), "[REDACTED_BY_ALCATRAZ_ANTHROPIC_KEY]"},
	{"openai_service_key", re(`\bsk-svcacct-[a-zA-Z0-9_-]{20,}\b`), "[REDACTED_BY_ALCATRAZ_OPENAI_SVC]"},
	{"google_key", re(`\bAIza[0-9A-Za-z_-]{35}\b`), "[REDACTED_BY_ALCATRAZ_GOOGLE_KEY]"},
	{"github_token", re(`\bghp_[a-zA-Z0-9]{36}\b`), "[REDACTED_BY_ALCATRAZ_GITHUB_TOKEN]"},
	{"github_token_old", re(`\bgho_[a-zA-Z0-9]{36}\b`), "[REDACTED_BY_ALCATRAZ_GITHUB_TOKEN]"},
	{"slack_token", re(`\bxox[baprs]-[0-9]{10,13}-[0-9]{10,13}(-[a-zA-Z0-9]{24})?\b`), "[REDACTED_BY_ALCATRAZ_SLACK_TOKEN]"},
	{"discord_token", re(`\b[a-zA-Z0-9_-]{24}\.[a-zA-Z0-9_-]{6}\.[a-zA-Z0-9_-]{27}\b`), "[REDACTED_BY_ALCATRAZ_DISCORD_TOKEN]"},
	{"aws_access_key", re(`\bAKIA[0-9A-Z]{16}\b`), "[REDACTED_BY_ALCATRAZ_AWS_KEY]"},
	{
		"aws_secret_key",
		re(`(?i)(?:"|')?(?:aws_secret_access_key|aws_secret|secret_key)(?:"|')?\s*['"]?\s*[:=]\s*['"]?[A-Za-z0-9/+=]{40}['"]?`),
		"[REDACTED_BY_ALCATRAZ_AWS_SECRET]",
	},
	{"jwt_token", re(`\beyJ[a-zA-Z0-9_-]*\.eyJ[a-zA-Z0-9_-]*\.[a-zA-Z0-9_-]*\b`), "[REDACTED_BY_ALCATRAZ_JWT]"},
	{
		"bearer_token",
		re(`(?i)(?:bearer|authorization)\s+['"]?[A-Za-z0-9_\-\.]{20,}['"]?`),
		"[REDACTED_BY_ALCATRAZ_BEARER]",
	},
	{"stripe_secret_key", re(`\bsk_(live|test)_[a-zA-Z0-9]{24,}\b`), "[REDACTED_BY_ALCATRAZ_STRIPE_KEY]"},
	{"stripe_publishable", re(`\bpk_(live|test)_[a-zA-Z0-9]{24,}\b`), "[REDACTED_BY_ALCATRAZ_STRIPE_KEY]"},
	{"stripe_restricted", re(`\brk_(live|test)_[a-zA-Z0-9]{24,}\b`), "[REDACTED_BY_ALCATRAZ_STRIPE_KEY]"},
	{"stripe_webhook", re(`\bwhsec_[a-zA-Z0-9]{32,}\b`), "[REDACTED_BY_ALCATRAZ_STRIPE_WEBHOOK]"},

	// ═══════════════════════════════════════════════════════════════════════
	// 1B. AI / LLM PROVIDERS
	// ═══════════════════════════════════════════════════════════════════════
	{"groq_key", re(`\bgsk_[a-zA-Z0-9]{40,}\b`), "[REDACTED_BY_ALCATRAZ_GROQ_KEY]"},
	{"perplexity_key", re(`\bpplx-[a-zA-Z0-9]{32,}\b`), "[REDACTED_BY_ALCATRAZ_PERPLEXITY_KEY]"},
	{"replicate_key", re(`\br8_[A-Za-z0-9]{37,}\b`), "[REDACTED_BY_ALCATRAZ_REPLICATE_KEY]"},
	{"huggingface_key", re(`\bhf_[a-zA-Z0-9]{34,}\b`), "[REDACTED_BY_ALCATRAZ_HUGGINGFACE_KEY]"},
	{"openrouter_key", re(`\bsk-or-v1-[a-f0-9]{64}\b`), "[REDACTED_BY_ALCATRAZ_OPENROUTER_KEY]"},
	{"cohere_key", re(`(?i)(?:"|')?(?:cohere[_\s]?api[_\s]?key|co_api_key)(?:"|')?\s*['"]?\s*[:=]\s*['"]?[A-Za-z0-9]{40}['"]?`), "[REDACTED_BY_ALCATRAZ_COHERE_KEY]"},
	{"mistral_key", re(`(?i)(?:"|')?(?:mistral[_\s]?api[_\s]?key)(?:"|')?\s*['"]?\s*[:=]\s*['"]?[A-Za-z0-9]{32}['"]?`), "[REDACTED_BY_ALCATRAZ_MISTRAL_KEY]"},

	// ═══════════════════════════════════════════════════════════════════════
	// 1C. CAPTCHA / ANTI-BOT / AUTOMATION
	// ═══════════════════════════════════════════════════════════════════════
	{
		"captcha_solver_key",
		re(`(?i)(?:"|')?(?:2captcha|rucaptcha|anticaptcha|anti[_\s-]?captcha|capmonster|capsolver|deathbycaptcha|captcha[_\s]?api[_\s]?key|captcha[_\s]?key)(?:"|')?\s*['"]?\s*[:=]\s*['"]?[a-zA-Z0-9]{20,}['"]?`),
		"[REDACTED_BY_ALCATRAZ_CAPTCHA_KEY]",
	},
	{"capsolver_key", re(`\bCAP-[A-Z0-9]{30,}\b`), "[REDACTED_BY_ALCATRAZ_CAPTCHA_KEY]"},
	{
		"proxy_credentials",
		re(`(?i)(?:"|')?(?:proxy[_\s]?(?:user|username|pass|password|auth|key))(?:"|')?\s*['"]?\s*[:=]\s*['"]?[^\s'";,$]{6,}['"]?`),
		"[REDACTED_BY_ALCATRAZ_PROXY_CRED]",
	},

	// ═══════════════════════════════════════════════════════════════════════
	// 1D. GIT / PACKAGES / CI
	// ═══════════════════════════════════════════════════════════════════════
	{"github_token_other", re(`\bgh[usr]_[A-Za-z0-9]{36}\b`), "[REDACTED_BY_ALCATRAZ_GITHUB_TOKEN]"},
	{"github_fine_grained", re(`\bgithub_pat_[A-Za-z0-9_]{82}\b`), "[REDACTED_BY_ALCATRAZ_GITHUB_TOKEN]"},
	{"gitlab_token", re(`\bglpat-[A-Za-z0-9_-]{20,}\b`), "[REDACTED_BY_ALCATRAZ_GITLAB_TOKEN]"},
	{"npm_token", re(`\bnpm_[A-Za-z0-9]{36}\b`), "[REDACTED_BY_ALCATRAZ_NPM_TOKEN]"},
	{"pypi_token", re(`\bpypi-[A-Za-z0-9_-]{16,}\b`), "[REDACTED_BY_ALCATRAZ_PYPI_TOKEN]"},
	{"docker_pat", re(`\bdckr_pat_[a-zA-Z0-9_-]{27,}\b`), "[REDACTED_BY_ALCATRAZ_DOCKER_PAT]"},
	{"atlassian_token", re(`\bATATT3[A-Za-z0-9_\-=]{100,}\b`), "[REDACTED_BY_ALCATRAZ_ATLASSIAN]"},

	// ═══════════════════════════════════════════════════════════════════════
	// 1E. EMAIL / SMS / NOTIFICATIONS
	// ═══════════════════════════════════════════════════════════════════════
	{"sendgrid_key", re(`\bSG\.[A-Za-z0-9_-]{22}\.[A-Za-z0-9_-]{43}\b`), "[REDACTED_BY_ALCATRAZ_SENDGRID_KEY]"},
	{"mailgun_key", re(`\bkey-[0-9a-f]{32}\b`), "[REDACTED_BY_ALCATRAZ_MAILGUN_KEY]"},
	{"mailchimp_key", re(`\b[0-9a-f]{32}-us[0-9]{1,2}\b`), "[REDACTED_BY_ALCATRAZ_MAILCHIMP_KEY]"},
	{"postmark_token", re(`(?i)(?:"|')?(?:postmark[_\s]?(?:server|account)?[_\s]?token)(?:"|')?\s*['"]?\s*[:=]\s*['"]?[0-9a-f-]{36}['"]?`), "[REDACTED_BY_ALCATRAZ_POSTMARK]"},
	{"twilio_account_sid", re(`\bAC[0-9a-f]{32}\b`), "[REDACTED_BY_ALCATRAZ_TWILIO_SID]"},
	{"twilio_api_key", re(`\bSK[0-9a-f]{32}\b`), "[REDACTED_BY_ALCATRAZ_TWILIO_KEY]"},
	{"telegram_bot_token", re(`\b\d{8,10}:[A-Za-z0-9_-]{35}\b`), "[REDACTED_BY_ALCATRAZ_TELEGRAM_TOKEN]"},
	{"sentry_dsn", re(`\bhttps://[0-9a-f]{32}@[a-z0-9.-]+/[0-9]+\b`), "[REDACTED_BY_ALCATRAZ_SENTRY_DSN]"},
	{"newrelic_key", re(`\bNRAK-[A-Z0-9]{27}\b`), "[REDACTED_BY_ALCATRAZ_NEWRELIC_KEY]"},

	// ═══════════════════════════════════════════════════════════════════════
	// 1F. E-COMMERCE / PAYMENTS / SaaS
	// ═══════════════════════════════════════════════════════════════════════
	{"shopify_token", re(`\bshp(at|ca|pa|ss)_[a-fA-F0-9]{32}\b`), "[REDACTED_BY_ALCATRAZ_SHOPIFY_TOKEN]"},
	{"square_token", re(`\bsq0(atp|csp)-[0-9A-Za-z_-]{22,43}\b`), "[REDACTED_BY_ALCATRAZ_SQUARE_TOKEN]"},
	{"linear_key", re(`\blin_api_[A-Za-z0-9]{40,}\b`), "[REDACTED_BY_ALCATRAZ_LINEAR_KEY]"},
	{"notion_secret", re(`\b(?:secret_[A-Za-z0-9]{43}|ntn_[A-Za-z0-9]{36,})\b`), "[REDACTED_BY_ALCATRAZ_NOTION_SECRET]"},
	{"supabase_key", re(`\bsbp_[a-f0-9]{40}\b`), "[REDACTED_BY_ALCATRAZ_SUPABASE_KEY]"},
	{"planetscale_token", re(`\bpscale_(tkn|pw)_[A-Za-z0-9_\-\.]{32,}\b`), "[REDACTED_BY_ALCATRAZ_PLANETSCALE]"},
	{"databricks_token", re(`\bdapi[a-f0-9]{32}\b`), "[REDACTED_BY_ALCATRAZ_DATABRICKS]"},
	{"vault_token", re(`\bhv[sb]\.[A-Za-z0-9]{24,}\b`), "[REDACTED_BY_ALCATRAZ_VAULT_TOKEN]"},

	// ═══════════════════════════════════════════════════════════════════════
	// 2. CLOUD CREDENTIALS
	// ═══════════════════════════════════════════════════════════════════════
	{
		"aws_account_id",
		re(`(?i)(?:"|')?(?:aws_account_id|account[_\s]?id|conta\s*aws)(?:"|')?\s*['"]?\s*[:=]\s*['"]?\d{12}['"]?`),
		"[REDACTED_BY_ALCATRAZ_AWS_ACCOUNT]",
	},
	{"aws_arn", re(`\barn:aws:[a-z0-9-]+:[a-z0-9-]*:\d{12}:[a-zA-Z0-9-_/:#+=,@\.]+\b`), "[REDACTED_BY_ALCATRAZ_AWS_ARN]"},
	{
		"aws_session_token",
		re(`(?i)(?:"|')?(?:aws_session_token|session_token)(?:"|')?\s*['"]?\s*[:=]\s*['"]?[A-Za-z0-9/+=]{100,}['"]?`),
		"[REDACTED_BY_ALCATRAZ_AWS_SESSION]",
	},
	{
		"azure_subscription",
		re(`(?i)(?:"|')?(?:subscription[_\s]?id|azure_subscription|azure_sub)(?:"|')?\s*['"]?\s*[:=]\s*['"]?[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}['"]?`),
		"[REDACTED_BY_ALCATRAZ_AZURE_SUB]",
	},
	{
		"azure_tenant",
		re(`(?i)(?:"|')?(?:tenant[_\s]?id|azure_tenant)(?:"|')?\s*['"]?\s*[:=]\s*['"]?[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}['"]?`),
		"[REDACTED_BY_ALCATRAZ_AZURE_TENANT]",
	},
	{
		"azure_client_secret",
		re(`(?i)(?:"|')?(?:client_secret|azure_secret)(?:"|')?\s*['"]?\s*[:=]\s*['"]?[A-Za-z0-9_\-]{20,}['"]?`),
		"[REDACTED_BY_ALCATRAZ_AZURE_SECRET]",
	},
	{
		"gcp_service_account",
		re(`(?i)(?:"|')?(?:private_key_id|client_email|project_id)(?:"|')?\s*['"]?\s*[:=]\s*['"]?[a-zA-Z0-9_-]{20,}['"]?`),
		"[REDACTED_BY_ALCATRAZ_GCP]",
	},
	{
		"gcp_oauth_client_id",
		re(`\b[0-9]{10,}-[a-z0-9]{32}\.apps\.googleusercontent\.com\b`),
		"[REDACTED_BY_ALCATRAZ_GCP_OAUTH]",
	},
	{"gcp_oauth_secret", re(`\bGOCSPX-[a-zA-Z0-9_-]{28}\b`), "[REDACTED_BY_ALCATRAZ_GCP_OAUTH_SECRET]"},
	{"gcp_oauth_access", re(`\bya29\.[0-9A-Za-z_-]{30,}\b`), "[REDACTED_BY_ALCATRAZ_GCP_ACCESS]"},
	{"firebase_fcm_key", re(`\bAAAA[A-Za-z0-9_-]{7}:[A-Za-z0-9_-]{140}\b`), "[REDACTED_BY_ALCATRAZ_FCM_KEY]"},
	{
		"cloudflare_global_key",
		re(`(?i)(?:"|')?(?:cloudflare|cf)[_\s]?(?:api[_\s]?)?(?:key|token|global[_\s]?key)(?:"|')?\s*['"]?\s*[:=]\s*['"]?[A-Za-z0-9_-]{37,}['"]?`),
		"[REDACTED_BY_ALCATRAZ_CLOUDFLARE]",
	},
	{"cloudflare_origin_ca", re(`\bv1\.0-[0-9a-f]{24}-[0-9a-f]{146}\b`), "[REDACTED_BY_ALCATRAZ_CLOUDFLARE_CA]"},
	{
		"azure_storage_conn",
		re(`(?i)DefaultEndpointsProtocol=https?;AccountName=[a-z0-9]+;AccountKey=[A-Za-z0-9+/=]{60,};?[^\s]*`),
		"[REDACTED_BY_ALCATRAZ_AZURE_STORAGE]",
	},
	{
		"azure_storage_key",
		re(`(?i)(?:"|')?(?:account_?key|storage_?key|azure_?storage)(?:"|')?\s*['"]?\s*[:=]\s*['"]?[A-Za-z0-9+/]{86,88}={0,2}['"]?`),
		"[REDACTED_BY_ALCATRAZ_AZURE_STORAGE]",
	},
	{"do_token", re(`\bdop_v1_[a-f0-9]{64}\b`), "[REDACTED_BY_ALCATRAZ_DO_TOKEN]"},
	{"terraform_token", re(`\btfrc_[A-Za-z0-9]{14}\.[A-Za-z0-9]{64}\b`), "[REDACTED_BY_ALCATRAZ_TF_TOKEN]"},
	{
		"k8s_secret",
		re(`(?i)(?:"|')?(?:kubeconfig|kubectl\s*secret|k8s[_\s]?token)(?:"|')?\s*['"]?\s*[:=]\s*['"]?[A-Za-z0-9+/=]{20,}['"]?`),
		"[REDACTED_BY_ALCATRAZ_K8S]",
	},

	// ═══════════════════════════════════════════════════════════════════════
	// 3. BRAZILIAN PII
	// ═══════════════════════════════════════════════════════════════════════
	{"cpf_formatted", re(`\b\d{3}\.\d{3}\.\d{3}-\d{2}\b`), "[REDACTED_BY_ALCATRAZ_CPF]"},
	{"cnpj_formatted", re(`\b\d{2}\.\d{3}\.\d{3}/\d{4}-\d{2}\b`), "[REDACTED_BY_ALCATRAZ_CNPJ]"},
	{
		"cpf_context",
		re(`(?i)(?:"|')?(?:cpf|cliente|titular|documento|pessoa\s*f[íi]sica)(?:"|')?\s*['"]?\s*[:=]\s*['"]?\d{3}\.?\d{3}\.?\d{3}-?\d{2}['"]?`),
		"[REDACTED_BY_ALCATRAZ_CPF]",
	},
	{
		"cnpj_context",
		re(`(?i)(?:"|')?(?:cnpj|empresa|raz[ãa]o\s*social|fornecedor|pessoa\s*jur[íi]dica)(?:"|')?\s*['"]?\s*[:=]\s*['"]?\d{2}\.?\d{3}\.?\d{3}/?\d{4}-?\d{2}['"]?`),
		"[REDACTED_BY_ALCATRAZ_CNPJ]",
	},
	{
		"rg_context",
		re(`(?i)(?:"|')?(?:rg|registro\s*geral|identidade)(?:"|')?\s*['"]?\s*[:=]\s*['"]?\d{1,2}\.?\d{3}\.?\d{3}-?[\dXx]?['"]?`),
		"[REDACTED_BY_ALCATRAZ_RG]",
	},
	{
		"brazilian_phone",
		re(`(?i)(?:"|')?(?:telefone|fone|celular|whatsapp|tel|contato)(?:"|')?\s*['"]?\s*[:=]\s*['"]?(?:\+?55\s?)?[\s-]?(?:\(?\d{2}\)?[\s-]?)?\d{4,5}[-\s]?\d{4}['"]?`),
		"[REDACTED_BY_ALCATRAZ_PHONE]",
	},
	{
		"pix_key",
		re(`(?i)(?:"|')?(?:chave\s*_?\s*pix|pix)(?:"|')?\s*['"]?\s*[:=]\s*['"]?[a-zA-Z0-9._-]{8,50}['"]?`),
		"[REDACTED_BY_ALCATRAZ_PIX]",
	},
	{
		"bank_account",
		re(`(?i)(?:"|')?(?:conta|ag[eê]ncia|n[úu]mero\s*da\s*conta)(?:"|')?\s*['"]?\s*[:=]\s*['"]?\d{4,20}['"]?`),
		"[REDACTED_BY_ALCATRAZ_BANK_ACCOUNT]",
	},

	// ═══════════════════════════════════════════════════════════════════════
	// 3B. COMPLIANCE — CHECKSUM-BACKED (Tier 1: aggressive, context-free)
	// Matched bare/unformatted; a structural validator (see patternValidators)
	// is the false-positive filter, so fake data with wrong check digits is
	// left untouched.
	// ═══════════════════════════════════════════════════════════════════════
	{"cpf_bare", re(`\b\d{11}\b`), "[REDACTED_BY_ALCATRAZ_CPF]"},
	{"pis_nis", re(`\b\d{3}\.?\d{5}\.?\d{2}-?\d\b`), "[REDACTED_BY_ALCATRAZ_PIS]"},
	{"cnpj_bare", re(`\b\d{14}\b`), "[REDACTED_BY_ALCATRAZ_CNPJ]"},
	{"cns_sus", re(`\b\d{3}[\s.]?\d{4}[\s.]?\d{4}[\s.]?\d{4}\b`), "[REDACTED_BY_ALCATRAZ_CNS]"},
	{"payment_card", re(`\b\d{4}(?:[\s-]?\d{4}){2}[\s-]?\d{1,7}\b`), "[REDACTED_BY_ALCATRAZ_CREDIT_CARD]"},
	{"payment_card_bare", re(`\b\d{13,19}\b`), "[REDACTED_BY_ALCATRAZ_CREDIT_CARD]"},
	{"iban", re(`\b[A-Z]{2}\d{2}[A-Z0-9]{11,30}\b`), "[REDACTED_BY_ALCATRAZ_IBAN]"},

	// ═══════════════════════════════════════════════════════════════════════
	// 3C. COMPLIANCE — CONTEXT-KEYED (Tier 2: no reliable checksum)
	// Fires only near a keyword, like the existing *_context patterns.
	// ═══════════════════════════════════════════════════════════════════════
	{
		"ssn_context",
		re(`(?i)(?:"|')?(?:ssn|social[\s_]?security(?:[\s_]?number)?)(?:"|')?\s*['"]?\s*[:=]\s*['"]?\d{3}-?\d{2}-?\d{4}['"]?`),
		"[REDACTED_BY_ALCATRAZ_SSN]",
	},
	{
		"swift_bic_context",
		re(`(?i)(?:"|')?(?:swift|bic)(?:[\s_]?code)?(?:"|')?\s*['"]?\s*[:=]\s*['"]?[A-Z]{6}[A-Z0-9]{2}(?:[A-Z0-9]{3})?['"]?`),
		"[REDACTED_BY_ALCATRAZ_SWIFT]",
	},
	{
		"cnh_context",
		re(`(?i)(?:"|')?(?:cnh|carteira\s*de\s*habilita[çc][ãa]o|registro\s*cnh)(?:"|')?\s*['"]?\s*[:=]\s*['"]?\d{11}['"]?`),
		"[REDACTED_BY_ALCATRAZ_CNH]",
	},
	{
		"titulo_eleitor_context",
		re(`(?i)(?:"|')?(?:t[íi]tulo\s*(?:de\s*)?eleitor(?:al)?)(?:"|')?\s*['"]?\s*[:=]\s*['"]?\d{4}\s?\d{4}\s?\d{4}['"]?`),
		"[REDACTED_BY_ALCATRAZ_TITULO]",
	},
	{
		"renavam_context",
		re(`(?i)(?:"|')?(?:renavam)(?:"|')?\s*['"]?\s*[:=]\s*['"]?\d{9,11}['"]?`),
		"[REDACTED_BY_ALCATRAZ_RENAVAM]",
	},
	{
		"plate_context",
		re(`(?i)(?:"|')?(?:placa)(?:"|')?\s*['"]?\s*[:=]\s*['"]?[A-Z]{3}-?\d[A-Z0-9]\d{2}['"]?`),
		"[REDACTED_BY_ALCATRAZ_PLATE]",
	},
	{
		"cep_context",
		re(`(?i)(?:"|')?(?:cep|c[óo]digo\s*postal)(?:"|')?\s*['"]?\s*[:=]\s*['"]?\d{5}-?\d{3}['"]?`),
		"[REDACTED_BY_ALCATRAZ_CEP]",
	},
	{
		"health_context",
		re(`(?i)(?:"|')?(?:prontu[áa]rio|laudo|diagn[óo]stico|cid-?10|\bcid\b)(?:"|')?\s*['"]?\s*[:=]\s*['"]?[^\s'";,]{2,60}['"]?`),
		"[REDACTED_BY_ALCATRAZ_HEALTH]",
	},
	{
		"seed_phrase",
		re(`(?i)(?:"|')?(?:seed[\s_]?phrase|mnemonic|recovery[\s_]?phrase|frase[\s_]?semente)(?:"|')?\s*['"]?\s*[:=]\s*['"]?(?:[a-z]+[\s,]+){11,}[a-z]+['"]?`),
		"[REDACTED_BY_ALCATRAZ_SEED_PHRASE]",
	},
	{
		"session_cookie",
		re(`(?i)(?:"|')?(?:set-cookie|session[_-]?id|sessionid|jsessionid|phpsessid|connect\.sid)(?:"|')?\s*['"]?\s*[:=]\s*['"]?[A-Za-z0-9%._+\-/]{16,}['"]?`),
		"[REDACTED_BY_ALCATRAZ_SESSION]",
	},

	// ═══════════════════════════════════════════════════════════════════════
	// 3C-i. INTERNATIONAL NATIONAL IDs (context-keyed + checksum)
	// Single-check-digit IDs — matched only near a keyword; the checksum
	// (see patternValidators) filters the residual false positives.
	// ═══════════════════════════════════════════════════════════════════════
	{
		"sin_context",
		re(`(?i)(?:"|')?\b(?:sin|social[\s_]?insurance(?:[\s_]?number)?|num[eé]ro\s*d'assurance\s*sociale)\b['"]?\s*(?:[:=]\s*|\s+)['"]?\d(?:[-.\s]?\d){8}['"]?`),
		"[REDACTED_BY_ALCATRAZ_SIN]",
	},
	{
		"imei_context",
		re(`(?i)(?:"|')?\b(?:imei)\b['"]?\s*(?:[:=]\s*|\s+)['"]?\d(?:[-.\s]?\d){14}['"]?`),
		"[REDACTED_BY_ALCATRAZ_IMEI]",
	},
	{
		"bsn_context",
		re(`(?i)(?:"|')?\b(?:bsn|burgerservicenummer|sofinummer)\b['"]?\s*(?:[:=]\s*|\s+)['"]?\d(?:[-.\s]?\d){8}['"]?`),
		"[REDACTED_BY_ALCATRAZ_BSN]",
	},
	{
		"nif_pt_context",
		re(`(?i)(?:"|')?\b(?:nif|n[uú]mero\s*de\s*identifica[çc][ãa]o\s*fiscal|contribuinte)\b['"]?\s*(?:[:=]\s*|\s+)['"]?\d(?:[-.\s]?\d){8}['"]?`),
		"[REDACTED_BY_ALCATRAZ_NIF]",
	},
	{
		"dni_es_context",
		re(`(?i)(?:"|')?\b(?:dni|nie|documento\s*nacional\s*de\s*identidad)\b['"]?\s*(?:[:=]\s*|\s+)['"]?\d(?:[-.\s]?\d){7}[-.\s]?[A-Za-z]['"]?`),
		"[REDACTED_BY_ALCATRAZ_DNI]",
	},
	{
		"aadhaar_context",
		re(`(?i)(?:"|')?\b(?:aadhaar|aadhar|uidai)\b['"]?\s*(?:[:=]\s*|\s+)['"]?\d(?:[-.\s]?\d){11}['"]?`),
		"[REDACTED_BY_ALCATRAZ_AADHAAR]",
	},

	// ═══════════════════════════════════════════════════════════════════════
	// 3D. INFRA / SECRETS IN URLs (context-free, distinctive formats)
	// ═══════════════════════════════════════════════════════════════════════
	{
		"db_connection_string",
		re(`(?i)\b(?:postgres|postgresql|mysql|mariadb|mongodb(?:\+srv)?|redis|rediss|amqp|amqps)://[^\s:@/'"]+:[^\s:@/'"]+@[^\s/'"]+`),
		"[REDACTED_BY_ALCATRAZ_DB_URL]",
	},
	{
		"basic_auth_url",
		re(`(?i)\bhttps?://[^\s:@/'"]+:[^\s:@/'"]+@[^\s/'"]+`),
		"[REDACTED_BY_ALCATRAZ_BASIC_AUTH]",
	},
	{
		"slack_webhook",
		re(`\bhttps://hooks\.slack\.com/services/T[A-Z0-9]+/B[A-Z0-9]+/[A-Za-z0-9]{20,}`),
		"[REDACTED_BY_ALCATRAZ_SLACK_WEBHOOK]",
	},
	{
		"discord_webhook",
		re(`\bhttps://(?:discord|discordapp)\.com/api/webhooks/\d+/[A-Za-z0-9_-]{20,}`),
		"[REDACTED_BY_ALCATRAZ_DISCORD_WEBHOOK]",
	},
	{
		"otpauth_uri",
		re(`\botpauth://[a-z]+/[^\s'"]*secret=[A-Za-z2-7]+[^\s'"]*`),
		"[REDACTED_BY_ALCATRAZ_OTP_SECRET]",
	},
	{"eth_private_key", re(`\b0x[a-fA-F0-9]{64}\b`), "[REDACTED_BY_ALCATRAZ_ETH_KEY]"},
	{
		"internal_hostname",
		re(`\b(?:[a-z0-9](?:[a-z0-9-]*[a-z0-9])?\.)+(?:local|internal|corp|intranet|lan)\b`),
		"[REDACTED_BY_ALCATRAZ_INTERNAL_HOST]",
	},

	// ═══════════════════════════════════════════════════════════════════════
	// 4. PII GLOBAL
	// ═══════════════════════════════════════════════════════════════════════
	{
		"email_context",
		re(`(?i)(?:"|')?(?:email|e-mail|usuario|login|contato|endere[cç]o)(?:"|')?\s*['"]?\s*[:=]\s*['"]?[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}['"]?`),
		"[REDACTED_BY_ALCATRAZ_EMAIL]",
	},
	{
		"email_strict",
		re(`\b[a-zA-Z0-9._%+-]{3,}@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}\b`),
		"[REDACTED_BY_ALCATRAZ_EMAIL]",
	},
	{
		"credit_card",
		re(`(?i)(?:"|')?(?:cart[ãa]o|card|cc|cr[eé]dito|d[eé]bito|n[úu]mero\s*do\s*cart[ãa]o)(?:"|')?\s*['"]?\s*[:=]\s*['"]?\d{4}[\s-]?\d{4}[\s-]?\d{4}[\s-]?\d{4}['"]?`),
		"[REDACTED_BY_ALCATRAZ_CREDIT_CARD]",
	},
	{
		"ip_address",
		re(`(?i)(?:"|')?(?:ip|endere[cç]o\s*ip|host)[A-Za-z0-9_]*(?:"|')?\s*['"]?\s*[:=]\s*['"]?\b(?:\d{1,3}\.){3}\d{1,3}\b['"]?`),
		"[REDACTED_BY_ALCATRAZ_IP]",
	},
	{
		"passport",
		re(`(?i)(?:"|')?(?:passaporte|passport|numero\s*do\s*passaporte)(?:"|')?\s*['"]?\s*[:=]\s*['"]?[A-Z]{2}\d{6,9}['"]?`),
		"[REDACTED_BY_ALCATRAZ_PASSPORT]",
	},

	// ═══════════════════════════════════════════════════════════════════════
	// 5. CRYPTOGRAPHIC KEYS
	// ═══════════════════════════════════════════════════════════════════════
	{
		"ssh_private_key",
		re(`(?s)-----BEGIN (?:OPENSSH|RSA|ECDSA|DSA|ED25519) PRIVATE KEY-----.*?-----END (?:OPENSSH|RSA|ECDSA|DSA|ED25519) PRIVATE KEY-----`),
		"[REDACTED_BY_ALCATRAZ_PRIVATE_KEY]",
	},
	{
		"pgp_private_key",
		re(`(?s)-----BEGIN PGP PRIVATE KEY BLOCK-----.*?-----END PGP PRIVATE KEY BLOCK-----`),
		"[REDACTED_BY_ALCATRAZ_PRIVATE_KEY]",
	},
	{
		"gpg_private_key",
		re(`(?s)-----BEGIN GPG PRIVATE KEY BLOCK-----.*?-----END GPG PRIVATE KEY BLOCK-----`),
		"[REDACTED_BY_ALCATRAZ_PRIVATE_KEY]",
	},
	{
		"gpg_public_key",
		re(`(?s)-----BEGIN PGP PUBLIC KEY BLOCK-----.*?-----END PGP PUBLIC KEY BLOCK-----`),
		"[REDACTED_BY_ALCATRAZ_PUBLIC_KEY]",
	},
	{
		"generic_private_key",
		re(`(?i)(?:"|')?(?:private[_\s]key|secret[_\s]key|client[_\s]secret)(?:"|')?\s*['"]?\s*[:=]\s*['"]?[A-Za-z0-9+/=]{20,}['"]?`),
		"[REDACTED_BY_ALCATRAZ_PRIVATE_KEY]",
	},

	// ═══════════════════════════════════════════════════════════════════════
	// 6. ENV & CONFIGURATION
	// ═══════════════════════════════════════════════════════════════════════
	{
		"env_secret",
		re(`(?i)(?:^|\n|\\n)[A-Z_]*(?:SECRET|PASSWORD|TOKEN|PRIVATE_KEY|API_KEY|DB_PASS|DATABASE_URL|CONNECTION_STRING)[A-Z_]*\s*[:=]\s*['"]?([^\s\n;,$]{8,})['"]?`),
		"[REDACTED_BY_ALCATRAZ_ENV_SECRET]",
	},
	{
		"generic_secret",
		re(`(?i)(?:"|')?(?:password|secret|token|api_key|private_key|bearer|auth)(?:"|')?\s*['"]?\s*[:=]\s*['"]?([^\s'"';,$]{8,})['"]?`),
		"[REDACTED_BY_ALCATRAZ_GENERIC_SECRET]",
	},
	{
		"email_credential",
		re(`(?i)(?:"|')?(?:smtp|imap|pop3|email|mail)\s*[_\s]?(?:host|server|user|username|pass|password|port)(?:"|')?\s*['"]?\s*[:=]\s*['"]?[^\s'"';,$]+['"]?`),
		"[REDACTED_BY_ALCATRAZ_EMAIL_CRED]",
	},
}

var AIHostPatterns = map[string]string{
	`.*\.openai\.com`:                         "openai",
	`openai\.com`:                             "openai",
	`.*\.chatgpt\.com`:                        "openai",
	`chatgpt\.com`:                            "openai",
	`.*\.anthropic\.com`:                      "anthropic",
	`anthropic\.com`:                          "anthropic",
	`.*\.generativelanguage\.googleapis\.com`: "google",
	`generativelanguage\.googleapis\.com`:     "google",
	`.*\.aistudio\.googleapis\.com`:           "google",
	`.*\.opencode\.ai`:                        "opencode",
	`opencode\.ai`:                            "opencode",
	`.*\.models\.dev`:                         "openrouter",
	`models\.dev`:                             "openrouter",
	`.*\.cohere\.ai`:                          "cohere",
	`cohere\.ai`:                              "cohere",
	`.*\.mistral\.ai`:                         "mistral",
	`mistral\.ai`:                             "mistral",
}

var compiledAIHosts []aiHostEntry

type aiHostEntry struct {
	re   *regexp.Regexp
	name string
}

func init() {
	compiledAIHosts = make([]aiHostEntry, 0, len(AIHostPatterns))
	for pattern, name := range AIHostPatterns {
		compiledAIHosts = append(compiledAIHosts, aiHostEntry{
			re:   regexp.MustCompile(pattern),
			name: name,
		})
	}
}

func re(pattern string) *regexp.Regexp {
	return regexp.MustCompile(pattern)
}

func DetectProvider(host string) string {
	for _, entry := range compiledAIHosts {
		if entry.re.MatchString(host) {
			return entry.name
		}
	}
	return "unknown"
}
