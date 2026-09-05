# flaretunnel-manager

`flaretunnel-manager` est une couche d’orchestration Go pour le fork public [johndoe237/FlareTunnel](https://github.com/johndoe237/FlareTunnel). Le manager interroge Cloudflare pour connaître l’état réel, puis délègue la création, la suppression ou l’utilisation des Workers à FlareTunnel.

## Image unique et version du fork

Le `Dockerfile` produit une image unique utilisable telle quelle en Docker local, sur un VPS ou sur un PaaS compatible avec les images Docker. Le fork est cloné et vérifié sur le commit exact `451990dcfc5abd1e698c4bf8aca312471799e07b`. Les trois fichiers de blacklist sont copiés dans `/opt/flaretunnel/` :

- `/opt/flaretunnel/blacklist-minimal.txt` (`minimal`, défaut) ;
- `/opt/flaretunnel/blacklist.txt` (`full`) ;
- `/opt/flaretunnel/blacklist-aggressive.txt` (`aggressive`).

Le choix se fait uniquement avec `FLARETUNNEL_BLACKLIST`. Il n’existe volontairement pas de variable utilisateur `FLARETUNNEL_BLACKLIST_DIR` : aucun chemin de blacklist arbitraire n’est accepté. Toute valeur de niveau inconnue retombe prudemment sur `minimal`.

Construire l’image :

```bash
docker build -t flaretunnel-manager:latest .
```

Le Dockerfile compile le package complet du fork avec `go build .`, et le manager avec `go build ./cmd/manager`. Aucun secret n’est incorporé dans l’image.

## Modes

| Mode | Variable JSON | Fonction |
|---|---|---|
| `create` | `CF_CREATE_ACCOUNTS` | Réconcilie la cible finale `target_workers` ; crée uniquement le manque constaté. |
| `delete` | `CF_DELETE_ACCOUNTS` | Supprime au maximum `target_workers` Workers `flaretunnel-*`. Cette valeur est un **nombre à supprimer**, pas une cible finale restante. |
| `use` | `CF_USE_ACCOUNTS` | Écrit une configuration multi-compte, exécute une seule opération `list`, conserve les endpoints, supprime les credentials temporaires, puis lance le tunnel. |

Les comptes sont traités séquentiellement. `create` et `delete` recomptent l’état réel avant chaque tentative, avec au maximum trois tentatives par compte, et continuent avec les comptes suivants après un échec.

En mode `delete`, l’appel automatisé est toujours borné :

```text
cleanup --account <ACCOUNT> --count <N> --yes
```

Le manager ne peut pas appeler l’ancien cleanup global sans `--count`. Si l’état réel est nul, ou si `target_workers` vaut zéro, aucun cleanup n’est lancé. Après chaque opération, la progression est calculée à partir de la différence réelle entre les deux comptages et plafonnée à la quantité demandée.

## Configuration

Variables prises en charge :

| Variable | Défaut | Description |
|---|---:|---|
| `MODE` | obligatoire | `create`, `delete` ou `use`. |
| `PORT` | `8080` | Port transmis à `tunnel`. |
| `FLARETUNNEL_MODE` | `random` | `random` ou `round-robin`. Valeur inconnue : `random`. |
| `FLARETUNNEL_BLACKLIST` | `minimal` | `minimal`, `full` ou `aggressive`. Valeur inconnue : `minimal`. |
| `CF_CREATE_ACCOUNTS` | — | Requis seulement en mode `create`. |
| `CF_DELETE_ACCOUNTS` | — | Requis seulement en mode `delete`. |
| `CF_USE_ACCOUNTS` | — | Requis seulement en mode `use`. |

Le JSON actif doit être un tableau non vide. Chaque entrée exige `name`, `api_token` et `account_id`. `target_workers` est obligatoire en `create` et `delete`, doit être entier et supérieur ou égal à zéro, et est ignoré en `use`. Une entrée invalide fait échouer toute la validation : aucun traitement partiel n’est réalisé. Les JSON des modes inactifs ne sont pas exigés.

Exemple :

```json
[{"name":"main","api_token":"TOKEN_INJECTE","account_id":"ACCOUNT_ID","target_workers":20}]
```

Ne mettez jamais de token réel dans Git, le README, les tests, les logs ou l’image. Utilisez `.env.example` comme modèle, puis un fichier `.env` non commité ou le gestionnaire de secrets de la plateforme.

## Docker local

```bash
cp .env.example .env
# Modifier .env hors du dépôt avec les vrais secrets
docker build -t flaretunnel-manager .
docker run --rm --env-file .env -p 8080:8080 flaretunnel-manager
```

Pour `create` ou `delete`, utilisez respectivement `CF_CREATE_ACCOUNTS` ou `CF_DELETE_ACCOUNTS` dans `.env`. En `use`, le tunnel est long-running et devient le processus principal du conteneur grâce à `exec`; arrêtez-le avec `docker stop`. Les éventuels fichiers montés doivent être configurés par la plateforme, mais les blacklists officielles restent celles embarquées dans `/opt/flaretunnel`.

## VPS

Installez Docker ou un runtime OCI compatible sur le VPS, puis récupérez l’image publiée ou construisez-la depuis ce dépôt. Injectez les variables avec un fichier protégé (`chmod 600 .env`) ou le mécanisme de secrets de votre système, sans les inscrire dans l’image ni la commande shell persistée :

```bash
docker pull IMAGE_PUBLIEE
# ou : docker build -t flaretunnel-manager .
docker run -d --name flaretunnel-manager --restart unless-stopped \
  --env-file /chemin/protege/.env -p 8080:8080 IMAGE_PUBLIEE
```

Aucun binaire Go n’a à être installé ou exécuté directement sur l’hôte : le VPS exécute l’image Docker. Pour `create` et `delete`, le conteneur termine lorsque l’opération est finie ; pour `use`, il reste actif comme service de tunnel.

## PaaS compatible Docker

Sélectionnez cette même image publiée ou le `Dockerfile`, définissez les secrets dans le gestionnaire de secrets du PaaS, configurez le processus principal sur l’ENTRYPOINT de l’image, et exposez le port `PORT` si le fournisseur le demande. Le health check peut vérifier la disponibilité du port en mode `use`. Ne supposez aucun fournisseur particulier et ne stockez pas les tokens dans des variables publiques ou dans le dépôt.

## Fichiers temporaires et sécurité

Les credentials sont écrits en `0600` dans un répertoire temporaire isolé par compte. En `use`, tous les comptes sont placés dans un unique fichier temporaire pour l’unique commande `list`; ce fichier est supprimé avant le lancement du tunnel, tandis que `flaretunnel_endpoints.json` est conservé. Les répertoires temporaires sont nettoyés à la fin du traitement. Les erreurs et sorties de processus masquent les secrets structurés et les valeurs sensibles présentes dans l’environnement.

## Tests et compilation

Les commandes suivantes compilent et vérifient le projet complet :

```bash
gofmt -w .
go test ./...
go vet ./...
go build ./cmd/manager
```

Un build Docker reproductible se vérifie avec :

```bash
docker build -t flaretunnel-manager:test .
```

## Licence et publication

La publication GitHub du projet est effectuée uniquement sur demande explicite. Vérifiez les licences du fork avant toute redistribution publique.
