# Référence des variables de déploiement

Ce document décrit les variables utilisées par `FlareTunnel-Manager` et par le processus FlareTunnel intégré à son image.

> Ne commitez jamais de vrais secrets. Utilisez le gestionnaire de secrets du PaaS ou du VPS, ou un fichier local protégé par les permissions du système.

## Fonctionnement général

Le manager est distribué dans une image Docker reproductible. Cette image contient le binaire FlareTunnel construit depuis le commit épinglé dans le `Dockerfile`, les fichiers de blacklist publics et les deux certificats publics. Elle ne contient aucune clé privée.

Les déploiements local, VPS et PaaS utilisent la même image. Le manager effectue l’orchestration Cloudflare, prépare les artefacts runtime et remplace son propre processus par FlareTunnel en mode `use`.

`FlareTunnel-Manager` et `omni-boot` sont indépendants. Le manager fournit le proxy ; omni-boot est déployé séparément et se connecte au proxy par le contrat réseau HTTPS.

## Modes

### `MODE=create`

Crée suffisamment de Workers nommés `flaretunnel-*` pour atteindre `target_workers` dans chaque compte.

### `MODE=delete`

Supprime le nombre demandé de Workers existants. La suppression est bornée et utilise toujours `cleanup --count N --yes`.

### `MODE=use`

Découvre les Workers existants, prépare les fichiers runtime, génère le certificat serveur TLS transport et lance le proxy FlareTunnel de manière persistante.

## Variables générales

| Variable | Obligatoire | Valeur par défaut | Description |
| --- | --- | --- | --- |
| `MODE` | Oui | Aucune | `create`, `delete` ou `use`. |
| `PORT` | Non | `8080` | Port d’écoute du proxy. Entier entre `1` et `65535`. |
| `FLARETUNNEL_MODE` | Non | `random` | `random` ou `round-robin`. |
| `FLARETUNNEL_BLACKLIST` | Non | `minimal` | `minimal`, `full` ou `aggressive`. |
| `CF_API_BASE_URL` | Non | `https://api.cloudflare.com/client/v4` | URL Cloudflare alternative pour les tests contrôlés. |
| `FLARETUNNEL_BINARY` | Non | `flaretunnel` | Binaire FlareTunnel à lancer. Dans l’image officielle : `/usr/local/bin/flaretunnel`. |

## Variables de comptes Cloudflare

Une seule variable de comptes active doit être fournie selon `MODE`. Sa valeur est un tableau JSON non vide.

Chaque objet doit contenir `name`, `api_token` et `account_id`. `target_workers` est requis pour `create` et `delete`, mais pas pour `use`. `zone_id` est facultatif.

```env
CF_CREATE_ACCOUNTS=[{"name":"main","api_token":"CLOUDFLARE_API_TOKEN","account_id":"CLOUDFLARE_ACCOUNT_ID","target_workers":10}]
CF_DELETE_ACCOUNTS=[{"name":"main","api_token":"CLOUDFLARE_API_TOKEN","account_id":"CLOUDFLARE_ACCOUNT_ID","target_workers":5}]
CF_USE_ACCOUNTS=[{"name":"main","api_token":"CLOUDFLARE_API_TOKEN","account_id":"CLOUDFLARE_ACCOUNT_ID"}]
```

## Authentification du proxy

`AUTH_PROXY` est obligatoire en mode `use`. Il s’agit d’un objet JSON strict contenant un nom d’utilisateur et un mot de passe non vides.

```env
AUTH_PROXY={"username":"proxy-user","password":"REPLACE_WITH_A_STRONG_PASSWORD"}
```

Le manager convertit cette valeur en Base64 et fournit en interne `AUTH_PROXY_BASIC` au child FlareTunnel. `AUTH_PROXY_BASIC` ne doit pas être configurée manuellement.

## Certificats et TLS

Les deux autorités de certification restent indépendantes.

### CA MITM

La clé privée du CA MITM est obligatoire en mode `use` et doit correspondre au certificat public embarqué dans l’image.

```env
FLARETUNNEL_MITM_CA_KEY_B64=BASE64_ENCODED_RSA_PRIVATE_KEY
```

Le manager valide la paire, matérialise temporairement la clé avec les permissions `0600`, puis transmet uniquement les chemins de fichiers au child FlareTunnel.

### CA transport

La clé privée du CA transport est obligatoire en mode `use` et doit correspondre à `certs/Flaretunnel-TRANSPORT-CA.crt`.

```env
FLARETUNNEL_TRANSPORT_CA_KEY_B64=BASE64_ENCODED_RSA_PRIVATE_KEY
```

Le manager génère une nouvelle clé serveur et un nouveau certificat serveur à chaque démarrage. Il supprime la clé privée du CA transport dès que la signature est terminée. Le secret Base64 n’est jamais transmis au child.

### Identité du listener transport

`FLARETUNNEL_TLS_SAN` est obligatoire en mode `use`.

```env
FLARETUNNEL_TLS_SAN=proxy.example.com 203.0.113.42 2001:db8::42
```

Les valeurs sont séparées par des espaces. Chaque valeur est un nom DNS, une IPv4 ou une IPv6 valide. `0.0.0.0` et `::` sont refusées car elles représentent des adresses d’écoute non spécifiques, pas des identités de certificat.

Le manager transmet au child les chemins éphémères suivants :

```env
FLARETUNNEL_MITM_CA_CERT=/runtime/Flaretunnel-MITM-CA.crt
FLARETUNNEL_MITM_CA_KEY=/runtime/Flaretunnel-MITM-CA.key
FLARETUNNEL_TRANSPORT_CERT=/runtime/Flaretunnel-Transport.crt
FLARETUNNEL_TRANSPORT_KEY=/runtime/Flaretunnel-Transport.key
```

Ces variables de chemins sont générées par le manager et ne doivent pas être utilisées pour fournir la clé privée du CA transport.

## Variables internes à ne pas configurer

```text
AUTH_PROXY_BASIC
FLARETUNNEL_TRANSPORT_CERT
FLARETUNNEL_TRANSPORT_KEY
FLARETUNNEL_MITM_CA_CERT
FLARETUNNEL_MITM_CA_KEY
FLARETUNNEL_BLACKLIST_DIR
```

Les quatre variables de certificats et de clés de chemin sont créées par le manager à partir du runtime. `FLARETUNNEL_BLACKLIST_DIR` n’est pas supportée ; le niveau de blacklist sélectionne un fichier approuvé dans l’image.

## Exemple complet du mode `use`

```env
MODE=use
AUTH_PROXY={"username":"proxy-user","password":"REPLACE_WITH_A_STRONG_PASSWORD"}
PORT=8080
FLARETUNNEL_MODE=random
FLARETUNNEL_BLACKLIST=minimal
CF_USE_ACCOUNTS=[{"name":"main","api_token":"REPLACE_WITH_A_CLOUDFLARE_TOKEN","account_id":"REPLACE_WITH_A_CLOUDFLARE_ACCOUNT_ID"}]
FLARETUNNEL_MITM_CA_KEY_B64=REPLACE_WITH_MITM_RSA_PRIVATE_KEY_BASE64
FLARETUNNEL_TRANSPORT_CA_KEY_B64=REPLACE_WITH_TRANSPORT_RSA_PRIVATE_KEY_BASE64
FLARETUNNEL_TLS_SAN=proxy.example.com 203.0.113.42
```

## Docker, VPS et PaaS

Construction locale :

```bash
docker build -t flaretunnel-manager:latest .
docker run --rm --env-file .env -p 8080:8080 flaretunnel-manager:latest
```

Sur un VPS :

```bash
chmod 600 /secure/path/flaretunnel-manager.env
docker run -d \
  --name flaretunnel-manager \
  --restart unless-stopped \
  --env-file /secure/path/flaretunnel-manager.env \
  -p 8080:8080 \
  IMAGE_REFERENCE
```

Sur un PaaS, configurez les variables dans le secret manager de la plateforme. Le filesystem ne doit pas être considéré comme persistant. Le manager traite les clés et les credentials comme des artefacts temporaires.

## Sécurité

Ne désactivez jamais la validation TLS côté client. N’utilisez pas `rejectUnauthorized: false` ni `NODE_TLS_REJECT_UNAUTHORIZED=0`. Ne publiez jamais de clé privée, de valeur Base64 de clé, de certificat serveur éphémère ou de fichier `.env` réel.

Les tokens Cloudflare doivent avoir les permissions minimales nécessaires. Les clés privées des CA MITM et transport ne doivent jamais être mélangées.

## Vérification

```bash
gofmt -w .
go test ./...
go vet ./...
go build ./cmd/manager
git diff --check
```

Avec Docker disponible :

```bash
docker build -t flaretunnel-manager:test .
```

## Références

- [FlareTunnel](https://github.com/johndoe237/FlareTunnel)
- [Commit FlareTunnel épinglé](https://github.com/johndoe237/FlareTunnel/commit/b37ccf2c7f61c536e225107554c90f01b1735558)
- [Documentation Docker](https://docs.docker.com/)

## Section obligatoire des variables d’environnement

Chaque ligne ci-dessous correspond à une variable obligatoire. Les variables marquées conditionnelles sont obligatoires uniquement dans le mode indiqué.

```env
MODE=use
AUTH_PROXY={"username":"proxy-user","password":"REPLACE_WITH_A_STRONG_PASSWORD"}                 # obligatoire en MODE=use
CF_USE_ACCOUNTS=[{"name":"main","api_token":"CLOUDFLARE_API_TOKEN","account_id":"CLOUDFLARE_ACCOUNT_ID"}] # obligatoire en MODE=use
CF_CREATE_ACCOUNTS=[{"name":"main","api_token":"CLOUDFLARE_API_TOKEN","account_id":"CLOUDFLARE_ACCOUNT_ID","target_workers":10}] # obligatoire en MODE=create
CF_DELETE_ACCOUNTS=[{"name":"main","api_token":"CLOUDFLARE_API_TOKEN","account_id":"CLOUDFLARE_ACCOUNT_ID","target_workers":5}] # obligatoire en MODE=delete
FLARETUNNEL_MITM_CA_KEY_B64=BASE64_ENCODED_RSA_PRIVATE_KEY                           # obligatoire en MODE=use
FLARETUNNEL_TRANSPORT_CA_KEY_B64=BASE64_ENCODED_RSA_PRIVATE_KEY                      # obligatoire en MODE=use
FLARETUNNEL_TLS_SAN=proxy.example.com 203.0.113.42                                  # obligatoire en MODE=use
```
