# FlareTunnel-Manager

[English version](README.en.md)

`FlareTunnel-Manager` est une couche d’orchestration Go qui prépare et exécute FlareTunnel. Il gère les comptes Cloudflare, la création et la suppression bornées des Workers, la validation des secrets, la génération des certificats runtime et le lancement du proxy.

> Le manager et `omni-boot` sont deux déploiements séparés. Le manager construit une image qui contient FlareTunnel. `omni-boot` se déploie dans une autre image et se connecte au proxy par le réseau.

## Architecture

```text
Déploiement FlareTunnel-Manager
  ├─ binaire manager
  ├─ binaire FlareTunnel épinglé
  ├─ CA MITM public
  ├─ CA transport public
  └─ secrets injectés au runtime
       │
       └─ lance FlareTunnel comme processus principal

Déploiement omni-boot séparé
       │
       └─ HTTPS proxy → listener FlareTunnel
```

Le manager ne réimplémente pas le proxy. Il prépare le runtime, puis remplace son processus par FlareTunnel en mode `use`.

## Fonctionnalités

- Image Docker unique utilisable localement, sur VPS ou sur un PaaS.
- Construction de FlareTunnel depuis un commit Git épinglé.
- Modes `create`, `delete` et `use`.
- Validation stricte des tableaux JSON de comptes Cloudflare.
- Suppression bornée avec `cleanup --count N --yes`.
- Nettoyage des credentials temporaires.
- Authentification proxy fournie au child sous forme interne `AUTH_PROXY_BASIC`.
- Validation cryptographique séparée des CA MITM et transport.
- Génération d’un certificat serveur transport éphémère avec SAN DNS, IPv4 ou IPv6.
- Purge de la clé privée du CA transport après signature et avant lancement du child.

## Modes de fonctionnement

| Mode | Variable de comptes | Résultat |
| --- | --- | --- |
| `create` | `CF_CREATE_ACCOUNTS` | Atteint le nombre `target_workers` demandé. |
| `delete` | `CF_DELETE_ACCOUNTS` | Supprime au plus le nombre demandé de Workers existants. |
| `use` | `CF_USE_ACCOUNTS` | Découvre les endpoints, prépare TLS et lance le proxy long-running. |

Les comptes sont traités séquentiellement. Les opérations de création et de suppression relisent l’état Cloudflare avant chaque tentative et continuent avec les comptes suivants lorsqu’un compte échoue.

## Variables obligatoires

En mode `use`, les variables obligatoires sont :

```env
MODE=use
AUTH_PROXY={"username":"proxy-user","password":"REPLACE_WITH_A_STRONG_PASSWORD"}
CF_USE_ACCOUNTS=[{"name":"main","api_token":"CLOUDFLARE_API_TOKEN","account_id":"CLOUDFLARE_ACCOUNT_ID"}]
FLARETUNNEL_MITM_CA_KEY_B64=BASE64_ENCODED_RSA_PRIVATE_KEY
FLARETUNNEL_TRANSPORT_CA_KEY_B64=BASE64_ENCODED_RSA_PRIVATE_KEY
FLARETUNNEL_TLS_SAN=proxy.example.com 203.0.113.42
```

En mode `create`, utilisez `CF_CREATE_ACCOUNTS` avec `target_workers`. En mode `delete`, utilisez `CF_DELETE_ACCOUNTS` avec `target_workers` égal au nombre maximal à supprimer.

La liste complète et les exemples ligne par ligne se trouvent dans [`DEPLOYMENT_ENV.md`](DEPLOYMENT_ENV.md).

## Secrets et certificats

`AUTH_PROXY` est un objet JSON manager-only. Le manager le convertit en Base64 et transmet uniquement `AUTH_PROXY_BASIC` au processus FlareTunnel. Ne configurez pas `AUTH_PROXY_BASIC` vous-même.

Le CA MITM et le CA transport sont indépendants. Le manager valide chaque clé privée contre son certificat public embarqué. Il matérialise les clés uniquement dans le runtime avec les permissions `0600`.

En mode `use`, le manager génère une clé serveur et un certificat serveur transport pour chaque démarrage. Le certificat contient les SAN de `FLARETUNNEL_TLS_SAN`. La clé du CA transport est supprimée dès que la signature est terminée ; elle n’est jamais héritée par FlareTunnel.

## Image Docker

Le `Dockerfile` utilise une construction multi-stage :

1. clonage du dépôt FlareTunnel ;
2. vérification du commit exact `b37ccf2c7f61c536e225107554c90f01b1735558` ;
3. compilation de FlareTunnel ;
4. compilation du manager ;
5. assemblage d’une image Alpine minimale.

L’image contient les certificats publics et les trois fichiers de blacklist. Elle ne contient aucune clé privée.

Construction et exécution :

```bash
docker build -t flaretunnel-manager:latest .
docker run --rm --env-file .env -p 8080:8080 flaretunnel-manager:latest
```

Pour un VPS :

```bash
chmod 600 /secure/path/flaretunnel-manager.env
docker run -d \
  --name flaretunnel-manager \
  --restart unless-stopped \
  --env-file /secure/path/flaretunnel-manager.env \
  -p 8080:8080 \
  IMAGE_REFERENCE
```

Sur un PaaS, injectez les secrets dans le secret manager de la plateforme et exposez le port `PORT`. Ne comptez pas sur la persistance du filesystem.

## Fichiers runtime

Le manager crée un répertoire temporaire protégé. Les fichiers de credentials sont supprimés avant le lancement du tunnel. Les chemins transmis au child sont :

```env
FLARETUNNEL_MITM_CA_CERT=/runtime/Flaretunnel-MITM-CA.crt
FLARETUNNEL_MITM_CA_KEY=/runtime/Flaretunnel-MITM-CA.key
FLARETUNNEL_TRANSPORT_CERT=/runtime/Flaretunnel-Transport.crt
FLARETUNNEL_TRANSPORT_KEY=/runtime/Flaretunnel-Transport.key
```

Ces chemins ne sont pas des secrets à injecter directement. Ils sont générés par le manager dans son runtime.

## Sécurité

Ne publiez jamais de clé privée, de secret Base64, de certificat serveur éphémère ou de fichier `.env` réel. N’utilisez pas `rejectUnauthorized: false` ni `NODE_TLS_REJECT_UNAUTHORIZED=0`.

Les tokens Cloudflare doivent respecter le principe du moindre privilège. Une rotation de CA doit être coordonnée avec les clients qui font confiance au certificat public correspondant.

## Tests

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

## Structure

```text
cmd/manager/              Point d’entrée
internal/business/        Orchestration create/delete/use
internal/ca/              Validation CA et certificats transport
internal/cloudflare/      API Cloudflare
internal/config/          Variables et valeurs par défaut
internal/flaretunnel/     Contrat avec le binaire FlareTunnel
internal/runtime/         Cycle de vie du runtime temporaire
Dockerfile                Image reproductible
DEPLOYMENT_ENV.md         Référence détaillée des variables
```

## Références

- [FlareTunnel](https://github.com/johndoe237/FlareTunnel)
- [Commit FlareTunnel épinglé](https://github.com/johndoe237/FlareTunnel/commit/b37ccf2c7f61c536e225107554c90f01b1735558)
- [Documentation Docker](https://docs.docker.com/)

---

[Lire cette documentation en anglais](README.en.md)
