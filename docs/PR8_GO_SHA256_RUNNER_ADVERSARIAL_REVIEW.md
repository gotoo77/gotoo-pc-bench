# PR #8 - Runner SHA-256 natif Go - Revue adversariale

## Périmètre et preuves

- **Source historique inspectée** : `benchmark_go.go` fourni pour la campagne v6 i3 du 15 novembre 2026, appelé par `benchmark_pc_v6.sh` après compilation `go build -trimpath`.
- **Charge exacte** : 131072 tâches, chacune 64 KiB (8 GiB par profil), buffers initialisés avec `byte(i*131 + 17)`, index `uint32` en little-endian dans `buf[:4]`, `sha256.Sum256(buf)` puis XOR des condensats par worker et XOR final.
- **Paramètres** : `GOMAXPROCS` = nombre de workers ; 1, 4 et 8 workers. Canaux `tasks` tamponnés avec `workers*2` et `results` avec `workers`, comme la référence.
- **Référence de checksum** : `3e1ddd341ade2515c601c25ab72130f92b87670e1fc39663390fa1b126dc2de1`, observée dans les trois profils i3 du rapport v6.
- **Runner autonome** : charge exécutée dans un **processus enfant du binaire Go actuel** par un sous-commande interne à paramètres strictement limités. N'exige ni `go` sur le PATH ni programme externe `benchmark_go.go` à côté de l'application.
- **Intégration** : `--test go` et `--test cpu,memory,ffmpeg,go` avec ordre canonique CPU → mémoire → FFmpeg → Go ; verrou existant ; annulation Ctrl+C / timeout (120 s par profil) ; publication **atomique** d'un seul rapport après tous les succès.
- **Aucune modification** de `internal/report/json.go`, du schéma JSON v1, de l'API HTTP ni du frontend. Les clés `go.1`, `go.4`, `go.8`, la version Go et le checksum étaient déjà acceptés.

## Contrôles adversariaux

| Risque | Prévention / observation |
| --- | --- |
| SHA-256 « similaire » mais protocole différent | Calcul repris du source historique (tailles, indices, initialisation, XOR, GOMAXPROCS). Oracle séquentiel indépendant sur plusieurs tailles de travail ; trois profils testés. |
| Optimisation/raccourci qui fausse le débit | 131072 SHA-256 réellement réalisés par profil ; checksum attendu fixé ; pas de résultat figé servant de mesure. |
| Résultat d'un enfant corrompu ou mensonger | Vérification stricte des workers, `GOMAXPROCS`, nombre de tâches, 8589934592 octets, durée/débit et checksum. Sortie supplémentaire interdite. |
| Un worker renvoie un autre checksum | Rejet, aucun `Run` partiel retourné ni rapport publié. |
| Exécution concurrente ou fin brutale | Le verrou `flock` existant reste détenu pendant toute la campagne ; le travail Go est un sous-processus tué à l'annulation/timeout. |
| Échec du dernier runner après CPU/RAM/FFmpeg | L'orchestrateur retourne une erreur avant `WriteResult` : aucun JSON partiel. |
| Régression de comparaison avec les anciens rapports | `Go` enregistré au format `go version` historique : version du compilateur + `linux/amd64` ; même SHA mais version Go différente => lignes Go non comparables. |
| Fuite de données | Aucun fichier input, aucun chemin privé, aucune donnée utilisateur hachée ; charge purement synthétique, JSON metadata machine déjà exposées par les autres runners. |
| Régression multiplateforme | Linux seul pour exécuter ; compilations Windows/macOS conservées pour `list/compare/serve`. |

## Validations effectuées

- `go test -count=1 ./...` : **PASS**.
- `go vet ./...` : **PASS**.
- `go test -race -count=1 ./...` : **PASS**.
- `node --test tests/*.test.cjs` : **8/8 PASS**.
- `node --check` (deux scripts UI) : **PASS**.
- Builds CGO_ENABLED=0 Linux amd64, Windows amd64, macOS arm64 : **PASS**.
- Benchmark **réel Go SHA-256** sur l'environnement Linux de validation : 1/4/8 workers, chacun 8 GiB, **checksum identique à la référence i3** dans un rapport JSON à 3 mesures ; préflight et lecture CLI PASS. Version Go locale `go1.23.2 linux/amd64` (différente de la référence i3, donc performance non comparable).
- Smoke CLI combiné 11 mesures : **PASS**, avec CPU/FFmpeg/mémoire simulés, mais **charge SHA-256 réelle** ; un seul JSON valide, 11 clés attendues, checksum historique, pas de fuite de chemin vidéo.
- GitHub Actions sur SHA exact : **à vérifier après publication**.
- Recette Fedora 44 sur l'i3 réel avec le compilateur système : **à effectuer avant fusion**.

## Limites connues

1. **Version Go != identité complète d'un protocole**. Le schéma JSON v1 n'inclut pas de `protocol_id`, ni flags exacts du compilateur, ni cadence CPU/thermique. Conserver les anciens rapports et refuser le gain Go si la version enregistrée diverge. Une version égale n'atteste pas d'un environnement rigoureusement identique.
2. **Même checksum ne prouve pas un débit comparable**. Le checksum démontre que la charge est sémantiquement identique ; pas que deux compilateurs ou environnements sont comparables en performance.
3. **Durée arrondie**. Le temps mesuré au sein du sous-processus est arrondi au millième comme le programme original, et le débit (MiB/s) à deux décimales. La création du sous-processus n'est pas incluse dans le temps chronométré.
4. **Performance et environnement**. Le runner impose `GOMAXPROCS` à 1/4/8 dans le processus enfant. La contention système, l'affinité CPU, le throttling et les changements de version Go affectent les résultats. Trois répétitions/médianes sont hors périmètre.
5. **Sous-processus auxiliaire**. La commande interne `__go-sha256-worker` est accessible à tout utilisateur local qui peut déjà exécuter le binaire. Ses arguments sont limités à 1/4/8 workers et à une charge fixe. Aucun endpoint HTTP ne l'invoque.
6. **Timeout de 120 s** par profil : une machine nettement plus lente que la référence peut échouer au lieu de produire un résultat, volontairement sans rapport partiel.
7. **Aucun système de plugins** : quatre runners fixes ; un registre générique ne se justifie pas encore par ce changement.

## Décision

**GO WITH CONDITIONS** : commit et CI GitHub, puis recette Fedora réelle sur l'i3 avec `--test go`, et si souhaité campagne complète à 11 mesures sur la vidéo historique avec SHA vérifié. Ne pas fusionner tant que le test Go réel sur Fedora n'est pas confirmé.
