# Gotoo PC Bench

**Gotoo PC Bench** est une application locale écrite en Go pour **consulter, analyser et comparer des campagnes de benchmark matériel** depuis une CLI et une interface Web.

**État actuel : visualisation, comparaison et runners CLI Linux CPU, mémoire et FFmpeg ; runner Go SHA-256 proposé dans la PR #8 (en recette).** Le projet importe les rapports historiques et les résultats JSON v1, et peut convertir les premiers vers le second. Le serveur Web reste **strictement en lecture seule**.

Les anciens scripts Bash ne sont **pas inclus dans le dépôt** : ils restent une source historique de rapports, sans être nécessaires aux nouveaux runners Go.

## Périmètre livré

- Une application Go, avec serveur HTTP local et ressources HTML/CSS/JS intégrées au binaire via `embed`.
- Une CLI Linux exécutant quatre protocoles fixes, séparément ou ensemble : **CPU** (1/4/8 threads, 30 s, nombres premiers jusqu’à 10000), **mémoire en écriture** (1/4 threads, blocs 1 KiB, limite 1 TiB, 15 s) et **FFmpeg/libx264** (1/4/8 threads, source FFV1 640×360/30 fps/20 s, preset medium, CRF 23, sortie null) et **Go SHA-256** (1/4/8 workers, 131072 blocs de 64 KiB, soit 8 GiB par profil), puis enregistrant un rapport JSON v1.
- Aucun framework ou service externe, aucune base SQL, aucun réseau sortant.
- Lecture des fichiers `rapport_*.txt` et `rapport_*.json` du dossier choisi, affichage de la configuration CPU/RAM/OS et des valeurs de CPU, RAM, FFmpeg, Go.
- Affichage d'un rapport seul (utile avant l'upgrade) ou comparaison de deux rapports.
- Pour chaque mesure comparable, calcul du gain en pourcentage en tenant compte du sens de la métrique (débit vs durée).
- Vérifications de comparabilité : vidéo source SHA-256, version FFmpeg, version Sysbench, checksum Go et version Go.
- API strictement **en lecture seule** : `GET /api/runs` et `GET /api/compare?before=...&after=...`.
- Aucun identifiant Machine ID ou Boot ID issu des logs n'est exporté.

**Non livré actuellement** : lancer/interrompre un benchmark depuis le navigateur, installer des dépendances, collecter la température, exécuter GPU/stockage depuis Go, calculer automatiquement des médianes.

## UX du tableau de bord (PR #3 fusionnée)

- Les campagnes sont identifiées par leur date, leur label, leur CPU, leur nombre de mesures et leur nom de fichier.
- Une seule campagne est affichée comme référence tant qu'aucune seconde campagne distincte n'a été sélectionnée. **Aucun gain avant/après n'est calculé automatiquement** entre deux rapports portant le même label.
- Les métriques sont regroupées par famille et les comparaisons dessinent deux barres de **performance relative** (avant = 100), en plus des valeurs brutes. Pour les métriques de durée (`s`), la performance est l'inverse de la durée, conformément au pourcentage fourni par le serveur.
- Les métriques non comparables n'ont pas de graphique de gain. La comparaison reste en lecture seule. Un avertissement signale les sélections inversées chronologiquement, et le bouton « Inverser » corrige leur ordre sans changer les rapports.
- Les tests JavaScript utilisent le runner natif `node:test` : `node --test tests/*.test.cjs` (Node nécessaire uniquement au développement, pas à l'exécution).

## Exécuter les benchmarks CPU, mémoire, FFmpeg et Go SHA-256 (Linux uniquement)

Sysbench est requis pour CPU/mémoire et FFmpeg avec `ffprobe` et l’encodeur **libx264** pour la vidéo. Le runner Go SHA-256 est intégré au binaire et n’exige **aucun compilateur Go externe** pour son exécution. Sur Fedora, `sudo dnf install sysbench` installe Sysbench ; la disponibilité de libx264 dépend du paquet FFmpeg installé. L'application ne l'installe jamais elle-même. La charge CPU sollicite fortement le processeur pendant environ 90 secondes ; la charge mémoire prend environ 30 secondes. Ces durées s’entendent hors préflight, compilation et écriture.

```bash
# Depuis la racine du dépôt
go run ./cmd/pc-bench check --test cpu
go run ./cmd/pc-bench run --test cpu --label cpu_probe --reports "$HOME/Téléchargements"
go run ./cmd/pc-bench check --test memory
go run ./cmd/pc-bench run --test memory --label memory_probe --reports "$HOME/Téléchargements"
# Une seule campagne contenant les cinq mesures CPU + RAM (environ 120 secondes) :
go run ./cmd/pc-bench check --test cpu,memory
go run ./cmd/pc-bench run --test cpu,memory --label combined_probe --reports "$HOME/Téléchargements"
go run ./cmd/pc-bench list --reports "$HOME/Téléchargements"

# Vidéo : utiliser l’ORIGINAL de la campagne historique (pas une copie régénérée).
VIDEO="$HOME/Téléchargements/benchmark_source_ffv1.mkv"
go run ./cmd/pc-bench check --test ffmpeg --video "$VIDEO"
go run ./cmd/pc-bench run --test ffmpeg --label video_probe --reports "$HOME/Téléchargements" --video "$VIDEO"
# Campagne CPU + RAM + FFmpeg, 8 métriques dans un JSON v1 unique :
go run ./cmd/pc-bench check --test cpu,memory,ffmpeg --video "$VIDEO"
go run ./cmd/pc-bench run --test cpu,memory,ffmpeg --label complete_probe --reports "$HOME/Téléchargements" --video "$VIDEO"

# SHA-256 uniquement : 3 profils × 8 GiB, sans dépendance Sysbench/FFmpeg :
go run ./cmd/pc-bench check --test go
go run ./cmd/pc-bench run --test go --label sha_probe --reports "$HOME/Téléchargements"

# Campagne complète historique, 11 mesures dans un JSON v1 unique :
go run ./cmd/pc-bench check --test cpu,memory,ffmpeg,go --video "$VIDEO"
go run ./cmd/pc-bench run --test cpu,memory,ffmpeg,go --label full_probe --reports "$HOME/Téléchargements" --video "$VIDEO"
```

- Les **mêmes paramètres CPU et mémoire** que les mesures historiques sont utilisés. La limite mémoire `1T` est un volume de transfert maximal, **pas une allocation de 1 TiB de RAM**. Il n’y a ni charge arbitraire ni installation privilégiée.
- Les campagnes CPU seules, mémoire seule ou CPU + mémoire (5 mesures) sont acceptées par le **même JSON v1**. Dans le mode combiné, le CPU s’exécute avant la RAM, même si les tests sont saisis dans l’autre ordre. Un sous-test échoué, un Ctrl+C ou un timeout ne publie aucun rapport.
- Un **verrou Linux partagé** empêche les runners CPU, mémoire, FFmpeg et Go de tourner simultanément dans un même répertoire de rapports. Le chemin de verrouillage v1 est conservé pour rester compatible avec les processus CPU plus anciens. Ctrl+C interrompt le runner et son sous-processus direct.
- Le JSON v1 n'intègre pas encore d'identifiant de protocole : la comparabilité des résultats exige donc de conserver les mêmes paramètres. Voir [revue PR #4](docs/PR4_CPU_RUNNER_ADVERSARIAL_REVIEW.md) et [revue PR #5](docs/PR5_MEMORY_RUNNER_ADVERSARIAL_REVIEW.md).
- `check` / `run` ne sont pas disponibles sur Windows/macOS. **La consultation des rapports reste portable.**


### Protocole Go SHA-256 et comparabilité

- Le runner reproduit le **fichier source historique `benchmark_go.go`** : 131072 tâches de 64 KiB, remplissage déterministe `byte(i*131 + 17)`, écriture de l'index `uint32` little-endian dans les quatre premiers octets, SHA-256 de chaque bloc, XOR des condensats par worker puis XOR final. `GOMAXPROCS` est égal à 1/4/8 selon le profil.
- Chaque profil traite **8 GiB**, sans créer de fichier disque ni allouer 8 GiB de RAM. La charge est CPU et mémoire, non un benchmark de vitesse de stockage. Les durées sont des temps réels arrondis à 3 décimales.
- Le travail est exécuté dans **un sous-processus du même binaire Go** pour être annulable par Ctrl+C ou timeout, sans dépendance à un compilateur externe. Le résultat doit correspondre au checksum historique `3e1ddd341ade2515c601c25ab72130f92b87670e1fc39663390fa1b126dc2de1` pour les trois profils, sinon aucune campagne n'est publiée.
- Le format JSON v1 conserve la **version Go complète et l'architecture** ainsi que le checksum. Une version de Go différente de la référence historique rend les durées **non comparables** dans l'IHM, même si les calculs produisent les mêmes octets. Les anciens scripts Bash ne sont pas intégrés au dépôt.

### Contrat vidéo et provenance

- La vidéo historique relevée sur Fedora 44 est **FFV1, 640×360, yuv420p, 30 fps, 20 s**, SHA-256 `ea7ca7ecd8795138031a703e324952653277928bea91208058dea7a6005c44d2`. Vérifier le SHA via la commande `check` avant de comparer les performances avec le i3.
- L’application **ne crée pas, ne convertit pas et ne modifie pas la source**. L'option `--video` est obligatoire dès que `ffmpeg` est sélectionné ; les liens symboliques et les fichiers hors protocole sont refusés. Le fichier est limité à 512 MiB, et son SHA est revalidé après l'encodage.
- Le préflight vérifie la présence de FFmpeg/FFprobe, l’existence effective de libx264 (mini-test d’encodage vers `null`), et le format de la source. Tous les préflights sont exécutés avant la campagne pour éviter d’effectuer les mesures CPU puis d’échouer faute d’encodeur vidéo.
- Comme dans le script v6 : lecture préalable du fichier vidéo pour réchauffer le cache, puis 1/4/8 encodages successifs avec `-nostdin -hide_banner -loglevel info -nostats -benchmark -i … -map 0:v:0 -an -sn -dn -c:v libx264 -preset medium -crf 23 -pix_fmt yuv420p -threads N -f null -`. Les temps en secondes correspondent à des **temps réels d'exécution**, arrondis au millième. Aucun fichier vidéo de sortie n'est créé.
- Les comparaisons FFmpeg sont marquées **non comparables** si l’empreinte SHA-256 de la vidéo ou la version FFmpeg ne concordent pas. Les builds x264 différents avec une version FFmpeg identique ne sont pas détectables par le schéma JSON v1 actuel : la comparabilité reste donc **indicative**, pas une preuve de protocole strictement identique.
- Les runtimes Windows/macOS conservent la lecture des rapports, mais pas ces runners. Pas de téléchargement, d'installation ni d’exécution via HTTP.

La revue des limites et de la comparabilité de cette tranche se trouve dans [PR7_FFMPEG_RUNNER_ADVERSARIAL_REVIEW.md](docs/PR7_FFMPEG_RUNNER_ADVERSARIAL_REVIEW.md).

## Démarrage du visualiseur Fedora (avec le binaire précompilé téléchargé séparément)

Télécharger le binaire `pc-bench-linux-amd64` puis, dans le dossier de téléchargement :

```bash
cd ~/Téléchargements
chmod +x pc-bench-linux-amd64
./pc-bench-linux-amd64 serve --reports "$PWD"
```

Ouvrir **http://127.0.0.1:8765** dans Firefox ou Chromium. Arrêter avec `Ctrl+C`.

Le binaire est compilé pour **Linux x86_64**, avec `CGO_ENABLED=0`. Ce binaire n'est pas un paquet Fedora signé. Si tu préfères une compilation locale contrôlée, utilise les sources ci-dessous.

## Compiler les sources

```bash
sudo dnf install golang
cd gotoo-pc-bench
go test ./...
go vet ./...
CGO_ENABLED=0 go build -trimpath -o pc-bench ./cmd/pc-bench
./pc-bench serve --reports "$HOME/Téléchargements"
```

Autres commandes :

```bash
./pc-bench list --reports "$HOME/Téléchargements"
./pc-bench compare --reports "$HOME/Téléchargements" \
  --before rapport_avant_upgrade_20261115_135844.txt \
  --after rapport_apres_upgrade_YYYYMMDD_HHMMSS.txt
```

La comparaison CLI utilise les **noms des rapports**, et non des chemins arbitraires.

## Invariants de sécurité

1. Le serveur refuse une adresse d'écoute non locale (`0.0.0.0`, adresse LAN...).
2. Seuls les fichiers réguliers `rapport_*.txt` ou `rapport_*.json` du répertoire choisi sont lus. Les liens symboliques sont ignorés.
3. Fichiers limités à 4 Mio, maximum 500 rapports. Les rapports JSON v1 sont validés ; un rapport historique sans marqueur final `FIN` reste affiché sans gain certifié.
4. Pas de lancement de commande depuis une requête HTTP. Les runners CPU/mémoire/FFmpeg/Go ne sont accessibles que par la CLI et ne requièrent aucun privilège root.
5. Les données de l'interface sont injectées via `textContent`, pas `innerHTML`.
6. En-têtes HTTP CSP restrictive, `nosniff`, `no-store` et `no-referrer`.

Le service n'est pas destiné à être exposé sur un réseau. Un utilisateur local ayant accès aux rapports peut déjà les lire directement ; ce programme ne constitue pas une barrière de sécurité entre utilisateurs locaux.

## Notes méthodologiques

- Les mesures `CPU` et `RAM` sont reconnues pour le protocole Sysbench de la v5/v6, et FFmpeg pour le protocole H.264 correspondant.
- Une source vidéo dont le SHA-256 diffère rend **FFmpeg non comparable**.
- Une version FFmpeg, Sysbench ou Go inconnue/différente rend les métriques concernées **non comparables** (indication de gain désactivée).
- Pour FFmpeg et Go, `+100 %` signifie **deux fois plus rapide en débit** (durée divisée par deux), pas une diminution de temps de 100 %.
- Les rapports v5/v6 ne contiennent pas de `protocol_id` explicite ni tous les paramètres possibles ; deux exécutions utilisant un script modifié hors convention peuvent malgré tout être considérées comme comparables. Une future version structurée devra ajouter `protocol_id` et paramètres de test.
- Une mesure unique peut être affectée par la charge système, les caches, la fréquence CPU ou la température. Comparer des médianes de trois essais constitue la prochaine étape de fiabilisation.
- Les versions ou builds différents de x264 à version FFmpeg identique ne sont pas détectés automatiquement. La comparabilité de l'encodeur est donc indicative.
- Les **données de référence i3** doivent être conservées avec le fichier vidéo FFV1 original ; le tableau de bord n'y touche jamais.

## Contrôles de validation

Sur l'environnement de développement :

- `go test ./...` : PASS. (Les validations historiques ne remplacent pas la CI et la recette spécifiques à PR #8.)
- `go vet ./...` : PASS.
- `node --check cmd/pc-bench/web/app.js` : PASS.
- `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build` : PASS.
- Import du rapport réel fourni `rapport_avant_upgrade` : PASS, **8 métriques reconnues**.
- Test HTTP avec serveur local et appel `GET /api/runs` : PASS.
- Tests anti-traversée de chemin, refus POST et refus de bind `0.0.0.0` : PASS.
- Vérification manuelle dans Firefox sur Fedora cible : **À FAIRE**.
- Démarrage Chromium headless dans le conteneur de développement : **non confirmé (blocage de l'environnement graphique)**.

## Étapes futures (non implémentées)

1. Documenter un `protocol_id` pour permettre de vérifier strictement la comparabilité des protocoles de mesures.
2. Ajouter trois exécutions et la comparaison des médianes, avec conditions de mesure.
3. Ajouter une orchestration Web explicitement autorisée, avec état de campagne et annulation. La sélection combinée CPU + mémoire + FFmpeg est déjà disponible uniquement en CLI.

## Export JSON v1 (PR #2)

La CLI peut exporter un rapport texte historique en document JSON v1, sans supprimer ni modifier l'original :

```bash
./pc-bench export --reports "$HOME/Téléchargements" --from rapport_avant_upgrade_20261115_142331.txt
```

L'importeur historique reste pris en charge. Lorsque les fichiers `.txt` et `.json` de même base coexistent, un JSON valide est prioritaire ; si le JSON est corrompu, le `.txt` est utilisé. Les chemins arbitraires et l'écrasement de fichiers existants sont refusés. Voir [contrat JSON v1](docs/RESULT_FORMAT_V1.md) et [schéma machine](docs/schema/result-v1.schema.json).

## PR #6 : orchestration minimale et ergonomie (fusionnée)

- `--test cpu,memory` regroupe cinq mesures dans **un seul rapport JSON v1**, enregistré seulement si les deux runners réussissent. `memory,cpu` est accepté mais l’ordre réel reste CPU puis mémoire.
- Le verrouillage, la publication atomique, les protocoles fixes et l’absence de commande HTTP restent inchangés.
- Les avertissements de provenance ne concernent que les familles de métriques **communes** aux deux campagnes. Le tableau de bord permet d’inverser la sélection avant/après et prévient d’une chronologie inversée.
- Aucun nouveau protocole ni module de plugin. Voir [revue adversariale PR #6](docs/PR6_ORCHESTRATION_ADVERSARIAL_REVIEW.md).
