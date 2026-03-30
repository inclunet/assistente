---
title: "Code Signing"
weight: 2
---

# Configuração de Assinatura de Código para Windows

## ✅ O que já foi configurado

1. **Informações do fornecedor** no `wails.json`:
   - CompanyName: Inclunet
   - ProductName: Assistente
   - Copyright e versão

2. **Workflow de assinatura** no `.github/workflows/release.yml`:
   - Step automático que assina executável e instalador
   - Usa signtool do Windows SDK

## 🔐 Como obter um certificado de assinatura de código

### Opção 1: Certificado Comercial (Recomendado para produção)

**Fornecedores confiáveis:**
- **DigiCert** - https://www.digicert.com/code-signing
- **Sectigo (Comodo)** - https://sectigo.com/ssl-certificates-tls/code-signing
- **GlobalSign** - https://www.globalsign.com/en/code-signing-certificate

**Preços:** Geralmente entre $200-$500/ano

**Processo:**
1. Comprar certificado do fornecedor
2. Verificação de identidade (pode levar 3-7 dias)
3. Receber arquivo `.pfx` (PKCS#12) com senha

### Opção 2: Certificado Auto-Assinado (Apenas para testes)

```powershell
# Criar certificado auto-assinado (Windows)
$cert = New-SelfSignedCertificate `
    -Type CodeSigningCert `
    -Subject "CN=Inclunet, O=Inclunet, C=BR" `
    -KeyAlgorithm RSA `
    -KeyLength 2048 `
    -Provider "Microsoft Enhanced RSA and AES Cryptographic Provider" `
    -CertStoreLocation "Cert:\CurrentUser\My" `
    -NotAfter (Get-Date).AddYears(2)

# Exportar para arquivo PFX
$password = ConvertTo-SecureString -String "SuaSenhaSegura" -Force -AsPlainText
Export-PfxCertificate -Cert $cert -FilePath "cert.pfx" -Password $password
```

⚠️ **Certificados auto-assinados geram avisos de segurança!**

## 🔧 Configurar GitHub Secrets

Você precisa adicionar 2 secrets no repositório:

### 1. Converter certificado para Base64

```powershell
# Windows PowerShell
$bytes = [System.IO.File]::ReadAllBytes("C:\caminho\para\cert.pfx")
$base64 = [System.Convert]::ToBase64String($bytes)
$base64 | Out-File -FilePath "cert_base64.txt"
```

Ou:

```bash
# Linux/macOS
base64 cert.pfx > cert_base64.txt
```

### 2. Adicionar secrets no GitHub

1. Vá para: **Settings → Secrets and variables → Actions**
2. Clique em **New repository secret**
3. Adicione:
   - **Nome:** `WINDOWS_CERTIFICATE_BASE64`
   - **Valor:** Conteúdo do arquivo `cert_base64.txt`

4. Adicione outro:
   - **Nome:** `WINDOWS_CERTIFICATE_PASSWORD`
   - **Valor:** A senha do certificado `.pfx`

## 🧪 Testar localmente

```powershell
# Testar assinatura local
signtool sign /f cert.pfx /p "senha" /tr http://timestamp.digicert.com /td sha256 /fd sha256 build/bin/assistente.exe

# Verificar assinatura
signtool verify /pa build/bin/assistente.exe
```

## ✅ Verificar se funcionou

Após configurar e fazer uma release:

1. Baixe o instalador
2. Clique com botão direito → **Propriedades**
3. Aba **Assinaturas Digitais**
4. Deve mostrar: "Inclunet" como assinante

## 🔄 Processo Completo

```bash
# 1. Fazer alterações no código
git add .
git commit -m "feat: nova funcionalidade"
git push

# 2. Criar release (dispara o workflow)
gh release create v1.0.20 --title "v1.0.20" --notes "Release notes"

# 3. Workflow vai:
#    - Buildar executável
#    - Buildar instalador NSIS
#    - Assinar ambos com o certificado (se configurado)
#    - Fazer upload para a release
```

## 📋 Checklist

- [ ] Obter certificado de assinatura de código
- [ ] Converter certificado para Base64
- [ ] Adicionar `WINDOWS_CERTIFICATE_BASE64` aos secrets
- [ ] Adicionar `WINDOWS_CERTIFICATE_PASSWORD` aos secrets
- [ ] Fazer uma release de teste
- [ ] Verificar assinatura digital no instalador baixado

## ⚠️ Notas Importantes

1. **Timestamp é crucial:** Usa `http://timestamp.digicert.com` para que a assinatura continue válida após o certificado expirar

2. **Custo:** Certificados comerciais custam dinheiro anualmente

3. **Validação:** Certificados auto-assinados funcionam mas geram avisos de segurança

4. **SmartScreen:** Mesmo com assinatura válida, o Windows Defender SmartScreen pode bloquear até o executável ganhar "reputação" (precisa de muitos downloads)

5. **Renovação:** Lembre de renovar o certificado antes de expirar e atualizar os secrets

## 🔗 Links Úteis

- [Microsoft Code Signing](https://docs.microsoft.com/en-us/windows/win32/seccrypto/cryptography-tools)
- [Wails Code Signing](https://wails.io/docs/guides/windows-installer)
- [SignTool Documentation](https://docs.microsoft.com/en-us/windows/win32/seccrypto/signtool)
