# PR #5 - Revue adversariale du runner mémoire Linux

## Objectif et état vérifié

À partir de `main` (`fc1a7c124017c3c66b7c0675f6d6707a0732c70e`), ajouter un **deuxième protocole CLI** réellement consommateur du mécanisme existant de préflight, d'annulation, de verrou et de résultat JSON v1. L'application ne possédait jusqu'ici qu'un runner CPU Sysbench. L'IHM et l'API HTTP restent en lecture seule.

## Contrat du benchmark mémoire

- Commande : `pc-bench check --test memory`, puis `pc-bench run --test memory --label LABEL --reports DIR`.
- Exécutable : Sysbench trouvé dans `PATH`, version lue via `sysbench --version`, Linux uniquement.
- **Options exactes et ordonnées** : `sysbench memory --memory-oper=write --memory-block-size=1K --memory-total-size=1T --threads=N --time=15 run`, pour `N=1`, puis `N=4`.
- Clés du schéma v1 : `ram.1` et `ram.4`, en `MiB/s`, `higher_is_better=true`, pour rester alignées avec les rapports historiques v6.
- La limite `1T` est un plafond de données transférées, **pas une allocation de 1 TiB**. La charge reste essentiellement une mesure Sysbench de petites écritures, et non une mesure exhaustive des débits DRAM physiques.
- Un timeout de 45 secondes protège chaque profil. Une sortie non nulle, manquante, dupliquée ou nulle provoque l'échec de la campagne entière. Aucun JSON partiel n'est publié.
- Le fichier JSON v1 est créé par la fonction existante `WriteResult` : mode `0600`, `Sync`, puis publication atomique sans écrasement.

## Analyse adversariale

| Risque | Protection actuelle | Limite résiduelle |
|---|---|---|
| Protocole divergent | Options Sysbench figées et assertions des commandes dans les tests | Le JSON v1 n'encode toujours pas `protocol_id` ni toutes les options ; même clé/version ne prouve pas une équivalence absolue |
| Faux débit (métrique tronquée, sortie malformée) | Une seule ligne de débit `MiB/sec`, valeur > 0, parse numérique stricte | Version de Sysbench, optimisations et caches influent sur les résultats |
| Durée réelle inférieure à 15 s | `--memory-total-size=1T` très élevé pour la machine cible | Une machine extrêmement rapide peut atteindre ce plafond avant 15 secondes ; aucune garantie universelle de durée sans une future vérification |
| Surcharge et exécution concurrente | `flock` partagé CPU/mémoire sur le fichier `.pc-bench-cpu.lock` historique | Verrou par dossier de rapports, pas global au système ; d'autres outils peuvent charger le CPU/RAM |
| Processus bloqué ou interruption | `exec.CommandContext`, timeout, propagation `Ctrl+C` | Les enfants indirects créés par un binaire externe ne sont pas gérés comme un groupe de processus |
| Résultat partiel au disque | Publication **après** réussite des deux sous-tests, validation JSON et nom sans écrasement | Si le système de fichiers ne permet pas les liens durs, la publication échoue explicitement |
| Fuite de données de machine | Collecte limitée CPU/RAM/OS/noyau, pas de machine-id/hostname dans le modèle | Les rapports matériels restent des données à examiner avant tout partage public |
| Faux succès du préflight | Vérifie présence, exécution de `--version`, système Linux et version reconnue | Ne prouve pas que la sous-commande `memory` est utilisable ; l'exécution elle-même est fail-closed |
| Extension incontrôlée | Aucune abstraction plugin, aucune modification JSON ou IHM | Le benchmark GPU et le stockage demandent un contrat de protocole à venir |
| Régression CPU | Protocole CPU inchangé ; même chemin de lock, `CheckCPU` conservé | Mesures réelles CPU non répétées localement après le changement de nom du verrou |

## Vérifications exécutées dans l'environnement de développement

- `go test -count=1 ./...` : PASS.
- `go vet ./...` : PASS.
- `go test -race -count=1 ./...` : PASS.
- `node --test tests/*.test.cjs` : PASS (7/7).
- Compilations Linux amd64, Windows amd64 et macOS arm64 : PASS, runner indisponible hors Linux.
- Faux exécutable Sysbench : options exactes, deux métriques, round-trip JSON v1 et lecture par le visualiseur, comparabilité avec les clés RAM historiques : PASS.
- Tests adversariaux : sorties absentes/doublées, débit nul, code retour non nul, échec au deuxième profil, interruption, label invalide et exclusion mutuelle CPU/mémoire : PASS.
- **Non exécuté ici** : benchmark Sysbench mémoire réel sur Fedora 44 et sa lecture dans l'IHM de la machine cible.

## Critères de fusion

1. CI GitHub verte sur le commit exact de la PR.
2. Fedora 44 : `check --test memory` passe avec Sysbench installé.
3. `run --test memory --label memory_probe` produit un nouveau JSON v1 contenant exactement `ram.1` et `ram.4`, relu via `list` et le tableau de bord.
4. Les campagnes historiques restent intactes, et le verrou interdit tout chevauchement des runners CPU et mémoire dans le même répertoire.

**Décision avant recette réelle : GO WITH CONDITIONS.** Pas de fusion automatique.
