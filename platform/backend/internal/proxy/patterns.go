package proxy

import (
	"regexp"
)

type SensitivePattern struct {
	Name        string
	Regex       *regexp.Regexp
	Replacement string
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
	// 1B. PROVEDORES DE IA / LLM
	// ═══════════════════════════════════════════════════════════════════════
	{"groq_key", re(`\bgsk_[a-zA-Z0-9]{40,}\b`), "[REDACTED_BY_ALCATRAZ_GROQ_KEY]"},
	{"perplexity_key", re(`\bpplx-[a-zA-Z0-9]{32,}\b`), "[REDACTED_BY_ALCATRAZ_PERPLEXITY_KEY]"},
	{"replicate_key", re(`\br8_[A-Za-z0-9]{37,}\b`), "[REDACTED_BY_ALCATRAZ_REPLICATE_KEY]"},
	{"huggingface_key", re(`\bhf_[a-zA-Z0-9]{34,}\b`), "[REDACTED_BY_ALCATRAZ_HUGGINGFACE_KEY]"},
	{"openrouter_key", re(`\bsk-or-v1-[a-f0-9]{64}\b`), "[REDACTED_BY_ALCATRAZ_OPENROUTER_KEY]"},
	{"cohere_key", re(`(?i)(?:"|')?(?:cohere[_\s]?api[_\s]?key|co_api_key)(?:"|')?\s*['"]?\s*[:=]\s*['"]?[A-Za-z0-9]{40}['"]?`), "[REDACTED_BY_ALCATRAZ_COHERE_KEY]"},
	{"mistral_key", re(`(?i)(?:"|')?(?:mistral[_\s]?api[_\s]?key)(?:"|')?\s*['"]?\s*[:=]\s*['"]?[A-Za-z0-9]{32}['"]?`), "[REDACTED_BY_ALCATRAZ_MISTRAL_KEY]"},

	// ═══════════════════════════════════════════════════════════════════════
	// 1C. CAPTCHA / ANTI-BOT / AUTOMAÇÃO
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
	// 1D. GIT / PACOTES / CI
	// ═══════════════════════════════════════════════════════════════════════
	{"github_token_other", re(`\bgh[usr]_[A-Za-z0-9]{36}\b`), "[REDACTED_BY_ALCATRAZ_GITHUB_TOKEN]"},
	{"github_fine_grained", re(`\bgithub_pat_[A-Za-z0-9_]{82}\b`), "[REDACTED_BY_ALCATRAZ_GITHUB_TOKEN]"},
	{"gitlab_token", re(`\bglpat-[A-Za-z0-9_-]{20,}\b`), "[REDACTED_BY_ALCATRAZ_GITLAB_TOKEN]"},
	{"npm_token", re(`\bnpm_[A-Za-z0-9]{36}\b`), "[REDACTED_BY_ALCATRAZ_NPM_TOKEN]"},
	{"pypi_token", re(`\bpypi-[A-Za-z0-9_-]{16,}\b`), "[REDACTED_BY_ALCATRAZ_PYPI_TOKEN]"},
	{"docker_pat", re(`\bdckr_pat_[a-zA-Z0-9_-]{27,}\b`), "[REDACTED_BY_ALCATRAZ_DOCKER_PAT]"},
	{"atlassian_token", re(`\bATATT3[A-Za-z0-9_\-=]{100,}\b`), "[REDACTED_BY_ALCATRAZ_ATLASSIAN]"},

	// ═══════════════════════════════════════════════════════════════════════
	// 1E. EMAIL / SMS / NOTIFICAÇÕES
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
	// 1F. E-COMMERCE / PAGAMENTOS / SaaS
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
	// 3. PII BRASILEIRO
	// ═══════════════════════════════════════════════════════════════════════
	{"cpf_formatado", re(`\b\d{3}\.\d{3}\.\d{3}-\d{2}\b`), "[REDACTED_BY_ALCATRAZ_CPF]"},
	{"cnpj_formatado", re(`\b\d{2}\.\d{3}\.\d{3}/\d{4}-\d{2}\b`), "[REDACTED_BY_ALCATRAZ_CNPJ]"},
	{
		"cpf_contexto",
		re(`(?i)(?:"|')?(?:cpf|cliente|titular|documento|pessoa\s*f[íi]sica)(?:"|')?\s*['"]?\s*[:=]\s*['"]?\d{3}\.?\d{3}\.?\d{3}-?\d{2}['"]?`),
		"[REDACTED_BY_ALCATRAZ_CPF]",
	},
	{
		"cnpj_contexto",
		re(`(?i)(?:"|')?(?:cnpj|empresa|raz[ãa]o\s*social|fornecedor|pessoa\s*jur[íi]dica)(?:"|')?\s*['"]?\s*[:=]\s*['"]?\d{2}\.?\d{3}\.?\d{3}/?\d{4}-?\d{2}['"]?`),
		"[REDACTED_BY_ALCATRAZ_CNPJ]",
	},
	{
		"rg_contexto",
		re(`(?i)(?:"|')?(?:rg|registro\s*geral|identidade)(?:"|')?\s*['"]?\s*[:=]\s*['"]?\d{1,2}\.?\d{3}\.?\d{3}-?[\dXx]?['"]?`),
		"[REDACTED_BY_ALCATRAZ_RG]",
	},
	{
		"telefone_br",
		re(`(?i)(?:"|')?(?:telefone|fone|celular|whatsapp|tel|contato)(?:"|')?\s*['"]?\s*[:=]\s*['"]?(?:\+?55\s?)?[\s-]?(?:\(?\d{2}\)?[\s-]?)?\d{4,5}[-\s]?\d{4}['"]?`),
		"[REDACTED_BY_ALCATRAZ_TELEFONE]",
	},
	{
		"chave_pix",
		re(`(?i)(?:"|')?(?:chave\s*_?\s*pix|pix)(?:"|')?\s*['"]?\s*[:=]\s*['"]?[a-zA-Z0-9._-]{8,50}['"]?`),
		"[REDACTED_BY_ALCATRAZ_PIX]",
	},
	{
		"conta_bancaria",
		re(`(?i)(?:"|')?(?:conta|ag[eê]ncia|n[úu]mero\s*da\s*conta)(?:"|')?\s*['"]?\s*[:=]\s*['"]?\d{4,20}['"]?`),
		"[REDACTED_BY_ALCATRAZ_CONTA_BANCARIA]",
	},

	// ═══════════════════════════════════════════════════════════════════════
	// 4. PII GLOBAL
	// ═══════════════════════════════════════════════════════════════════════
	{
		"email_contexto",
		re(`(?i)(?:"|')?(?:email|e-mail|usuario|login|contato|endere[cç]o)(?:"|')?\s*['"]?\s*[:=]\s*['"]?[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}['"]?`),
		"[REDACTED_BY_ALCATRAZ_EMAIL]",
	},
	{
		"email_estrito",
		re(`\b[a-zA-Z0-9._%+-]{3,}@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}\b`),
		"[REDACTED_BY_ALCATRAZ_EMAIL]",
	},
	{
		"cartao_credito",
		re(`(?i)(?:"|')?(?:cart[ãa]o|card|cc|cr[eé]dito|d[eé]bito|n[úu]mero\s*do\s*cart[ãa]o)(?:"|')?\s*['"]?\s*[:=]\s*['"]?\d{4}[\s-]?\d{4}[\s-]?\d{4}[\s-]?\d{4}['"]?`),
		"[REDACTED_BY_ALCATRAZ_CARTAO]",
	},
	{
		"endereco_ip",
		re(`(?i)(?:"|')?(?:ip|endere[cç]o\s*ip|host)[A-Za-z0-9_]*(?:"|')?\s*['"]?\s*[:=]\s*['"]?\b(?:\d{1,3}\.){3}\d{1,3}\b['"]?`),
		"[REDACTED_BY_ALCATRAZ_IP]",
	},
	{
		"passaporte",
		re(`(?i)(?:"|')?(?:passaporte|passport|numero\s*do\s*passaporte)(?:"|')?\s*['"]?\s*[:=]\s*['"]?[A-Z]{2}\d{6,9}['"]?`),
		"[REDACTED_BY_ALCATRAZ_PASSAPORTE]",
	},

	// ═══════════════════════════════════════════════════════════════════════
	// 5. CHAVES CRIPTOGRÁFICAS
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
	// 6. ENV & CONFIGURAÇÕES
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
	`.*\.openai\.com`:                        "openai",
	`.*\.anthropic\.com`:                     "anthropic",
	`.*\.generativelanguage\.googleapis\.com`: "google",
	`generativelanguage\.googleapis\.com`:     "google",
	`.*\.aistudio\.googleapis\.com`:           "google",
	`.*\.opencode\.ai`:                       "opencode",
	`.*\.models\.dev`:                        "openrouter",
	`.*\.cohere\.ai`:                         "cohere",
	`.*\.mistral\.ai`:                        "mistral",
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
