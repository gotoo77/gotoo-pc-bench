# Gotoo PC Bench - MVP v0.1

**Gotoo PC Bench** est une application locale écrite en Go pour **consulter, analyser et comparer des campagnes de benchmark matériel** depuis une CLI et une interface Web.

**État actuel : visualisation et comparaison uniquement.** Le projet importe les rapports texte historiques ainsi que les résultats JSON v1 et peut convertir les premiers vers le second. L'exécution des benchmarks depuis l'application n'est **pas encore implémentée**.

Les scripts Bash historiques ayant servi aux premières mesures ne sont **pas inclus dans ce dépôt** : ils restent des outils externes de compatibilité, pas le moteur du produit.

## Périmètre livré

- Une application Go, avec serveur HTTP local et ressources HTML/CSS/JS intégrées au binaire via `embed`.
- Aucun framework ou service externe, aucune base SQL, aucun réseau sortant.
- Lecture des fichiers `rapport_*.txt` et `rapport_*.json` du dossier choisi, affichage de la configuration CPU/RAM/OS et des valeurs de CPU, RAM, FFmpeg, Go.
- Affichage d'un rapport seul (utile avant l'upgrade) ou comparaison de deux rapports.
- Pour chaque mesure comparable, calcul du gain en pourcentage en tenant compte du sens de la métrique (débit vs durée).
- Vérifications de comparabilité : vidéo source SHA-256, version FFmpeg, version Sysbench, checksum Go et version Go.
- API strictement **en lecture seule** : `GET /api/runs` et `GET /api/compare?before=...&after=...`.
- Aucun identifiant Machine ID ou Boot ID issu des logs n'est exporté.

**Non livré actuellement** : lancer/interrompre un benchmark depuis le navigateur, installer des dépendances, collecter la température, produire des mesures JSON directement depuis un runner intégré, calculer automatiquement des médianes.

## UX du tableau de bord (PR #3, en cours de validation)

- Les campagnes sont identifiées par leur date, leur label, leur CPU, leur nombre de mesures et leur nom de fichier.
- Une seule campagne est affichée comme référence tant qu'aucune seconde campagne distincte n'a été sélectionnée. **Aucun gain avant/après n'est calculé automatiquement** entre deux rapports portant le même label.
- Les métriques sont regroupées par famille et les comparaisons dessinent deux barres de **performance relative** (avant = 100), en plus des valeurs brutes. Pour les métriques de durée (`s`), la performance est l'inverse de la durée, conformément au pourcentage fourni par le serveur.
- Les métriques non comparables n'ont pas de graphique de gain. La comparaison reste en lecture seule.
- Les tests JavaScript utilisent le runner natif `node:test` : `node --test tests/*.test.cjs` (Node nécessaire uniquement au développement, pas à l'exécution).

## Démarrage rapide Fedora (avec le binaire précompilé téléchargé séparément)

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
2. Seuls les fichiers réguliers `rapport_*.txt` et `rapport_*.json` du répertoire choisi sont lus. Les liens symboliques sont ignorés.
3. Fichiers limités à 4 Mio, maximum 500 rapports. Un rapport sans marqueur final `FIN` reste affiché, mais ses gains ne sont pas certifiés.
4. Pas de lancement de commande depuis une requête HTTP. Aucun privilège root requis.
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

- `go test ./...` : PASS.
- `go vet ./...` : PASS.
- `node --check cmd/pc-bench/web/app.js` : PASS.
- `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build` : PASS.
- Import du rapport réel fourni `rapport_avant_upgrade` : PASS, **8 métriques reconnues**.
- Test HTTP avec serveur local et appel `GET /api/runs` : PASS.
- Tests anti-traversée de chemin, refus POST et refus de bind `0.0.0.0` : PASS.
- Vérification manuelle dans Firefox sur Fedora cible : **À FAIRE**.
- Démarrage Chromium headless dans le conteneur de développement : **non confirmé (blocage de l'environnement graphique)**.

## Étapes futures (non implémentées)

1. Standardiser un export JSON atomique et versionné côté benchmark v6+, sans modifier la référence historique.
2. Ajouter trois exécutions et la comparaison des médianes, avec conditions de mesure.
3. Seulement ensuite, implémenter `run` et `cancel` côté Go avec liste de commandes autorisées, absence de root et état strict « une campagne à la fois ».

## Export JSON v1 (PR #2)

La CLI peut exporter un rapport texte historique en document JSON v1, sans supprimer ni modifier l'original :

```bash
./pc-bench export --reports "$HOME/Téléchargements" --from rapport_avant_upgrade_20261115_142331.txt
```

L'importeur historique reste pris en charge. Lorsque les fichiers `.txt` et `.json` de même base coexistent, un JSON valide est prioritaire ; si le JSON est corrompu, le `.txt` est utilisé. Les chemins arbitraires et l'écrasement de fichiers existants sont refusés. Voir [contrat JSON v1](docs/RESULT_FORMAT_V1.md) et [schéma machine](docs/schema/result-v1.schema.json).
