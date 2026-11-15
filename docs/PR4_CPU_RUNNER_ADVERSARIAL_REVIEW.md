# PR #4 - revue adversariale : runner CPU Linux

## Objectif et contrat

Intégrer **une seule** capacité d'exécution dans l'application Go : `pc-bench run --test cpu --label LABEL --reports DIR`. La CLI déclenche Sysbench CPU à 1, 4, puis 8 threads, avec `--cpu-max-prime=10000` et `--time=30`. Elle crée un rapport JSON **v1**, privé (`0600`), sans écraser un rapport existant. Un `check --test cpu` sans modification système permet le préflight.

Ce runner n'installe pas Sysbench, ne demande jamais root et n'ajoute **aucun endpoint HTTP d'exécution**. Le navigateur et le parseur historique restent en lecture seule. Il s'agit d'une première intégration Linux, et non d'un framework de plugins.

## Observations avant modification

- La CLI du dépôt possède `serve`, `list`, `compare` et `export`, mais aucun processus de mesure.
- Le format JSON v1 accepte `cpu.{1,4,8}`, la version Sysbench et les résultats partiels (1 à 64 métriques).
- Le protocole des scripts historiques fixe la limite des nombres premiers à **10000**, la durée à **30 s**, et les profils **1/4/8 threads**.
- Le comparateur signalait à tort une empreinte vidéo absente lorsqu'il comparait deux rapports ne comportant **que** des résultats CPU.

## Risques et décisions

| Risque | Protection implémentée | Limite résiduelle |
|---|---|---|
| Comparer des protocoles CPU différents | Options Sysbench figées, même clés et version d'outil enregistrée | JSON v1 ne contient pas encore un `protocol_id` vérifiable. Une modification extérieure de Sysbench à version égale reste possible. |
| Processus bloqué ou annulé | `exec.CommandContext`, timeout de 65 s par profil et propagation de Ctrl+C | Une commande externe peut créer ses propres processus enfants. |
| Benchmarks CPU concurrents | Verrou exclusif Linux (`flock`) pour les exécutions de cette CLI | Une charge CPU lancée par un autre outil reste possible. |
| Résultats incomplets ou erreurs | Aucun JSON n'est publié tant que les trois sous-tests n'ont pas réussi ; zéro et doublons refusés | Aucune répétition/médiane automatique. |
| Fichiers corrompus ou écrasés | Validation JSON v1, fichier temporaire privé, `sync`, publication par lien dur atomique non écrasant | Le système de fichiers doit permettre les liens durs dans le répertoire de rapports. |
| Fuite de données personnelles | Métadonnées système limitées à la CPU, RAM, OS et noyau ; pas de Machine ID, hostname ou chemins dans le JSON | Le nom du CPU et la configuration restent des métadonnées locales ; ne pas publier les rapports sans vérification. |
| Dérive de scope | Aucun registre de modules, aucune installation de paquets, aucune refonte UI, aucun changement de schéma | Stockage, GPU, températures et pilotage Web restent hors périmètre. |
| Faux avertissement FFmpeg | Avertissements sur la vidéo uniquement en présence de métriques FFmpeg | Un rapport CPU seul ne vérifie pas FFmpeg. |

## Validation exécutée

- `gofmt`, `go test -count=1 ./...`, `go vet ./...`, `go test -race -count=1 ./...` : PASS sur Linux.
- Builds croisés : Linux amd64, Windows amd64 et macOS arm64 : PASS. **Le runner CPU est volontairement refusé hors Linux**.
- Tests avec faux exécutable Sysbench dans un répertoire temporaire : les options **exactes** ont été observées ; sortie valide, sortie invalide, code non nul, annulation, timeout, version inconnue : couverts.
- Test verrou Linux : second lancement refusé, reprise possible après déverrouillage.
- Smoke CLI `check`, `run`, `list` avec Sysbench simulé : PASS, JSON v1 lu et permissions 0600.
- Les **mesures CPU réelles de 90 secondes sur Fedora 44 restent à exécuter**. Aucune performance de la machine cible n'est inventée.

## Critère de GO final

1. CI GitHub verte sur le SHA exact de la PR.
2. Sous Fedora 44, `check --test cpu` passe avec le vrai Sysbench.
3. `run --test cpu --label cpu_probe` produit trois métriques non nulles et le rapport apparaît dans `list` et le tableau de bord.
4. Aucune donnée i3 de référence existante n'est modifiée, et le serveur Web ne lance aucune commande.

**État : prêt pour recette Fedora, pas de fusion automatique.**
