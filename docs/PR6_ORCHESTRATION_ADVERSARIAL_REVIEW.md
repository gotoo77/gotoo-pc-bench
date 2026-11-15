# PR #6 - Revue adversariale : campagne CPU + mémoire, comparateur et chronologie

## État observé

- `main` au départ : `74765947da0801f5a6968edeb19a90914d687f3f` (PR #5 fusionnée).
- CPU et RAM sont deux runners Linux/Sysbench validés chacun sur Fedora 44, avec profils fixes et `flock` commun.
- Le JSON v1 accepte déjà les clés `cpu.N` et `ram.N` dans un même rapport.
- L'API `/api/compare` et la vue HTML comparent des campagnes choisies explicitement. Un avertissement vidéo pouvait apparaître si **une seule** campagne contenait FFmpeg, même sans métrique FFmpeg commune. La sélection pouvait être inversée dans le temps sans avertissement.

## Décision et périmètre

- Ajouter uniquement une sélection CLI `--test cpu,memory`, avec ordre de test déterministe CPU puis mémoire. Les modes historiques `cpu` et `memory` restent inchangés.
- Réutiliser **les deux runners existants** au lieu de modifier leurs protocoles. Le verrou est acquis une fois par la CLI ; une publication JSON n'est possible qu'une fois toutes les mesures réussies.
- Filtrer les avertissements au niveau des **familles réellement communes** : CPU/RAM, FFmpeg, Go.
- Dans l'IHM : avertissement de chronologie inversée et bouton « Inverser » accessible, sans changement silencieux du sens de la comparaison.
- Aucune modification du schéma JSON v1, ni commande d'exécution HTTP, ni runner FFmpeg/disque/GPU, ni installation de Sysbench, ni architecture de plugins.

## Menaces et contrôles

| Risque | Contrôle / limite |
| --- | --- |
| Le 2e runner échoue après 90 s CPU | `RunSelected` renvoie une erreur sans métriques ; la CLI n'appelle pas `WriteResult` |
| Ctrl+C ou timeout | Contexte transmis aux runners, vérifié entre sous-tests et avant publication |
| Deux processus concurrents | Verrou Linux conservé sur l'intégralité des deux runners et l'écriture |
| Profil inconnu, doublonné ou malformé | `ParseTests` refuse les valeurs hors `cpu`, `memory`, `cpu,memory` (ou `memory,cpu`) |
| Incohérence Sysbench entre runners | Refus d'une campagne combinée si la version rapportée change |
| Rapport JSON v1 incompatible | Clés, unités, directions et champs identiques aux runners individuels ; round-trip testé |
| Avertissement FFmpeg non pertinent | Activation uniquement si les deux campagnes possèdent une clé FFmpeg identique |
| Sélection avant/après inversée | Avertissement sur timestamps RFC3339 comparés en instants absolus ; bouton explicite |
| Compromission du host via le Web | API et CSP inchangées, HTTP strictement lecture seule |

## Validation exigée

- Tests stricts de sélection, ordre, compatibilité anciennes commandes et refus des doublons.
- Tests des cinq métriques (`cpu.1`, `.4`, `.8`, `ram.1`, `.4`) avec faux Sysbench, puis lecture JSON.
- Échec du runner mémoire après le CPU : **zéro nouveau rapport**.
- Tests du comparateur CPU seul face à une campagne complète, et FFmpeg réellement commun.
- Tests des timestamps et validation navigateur pour l'inversion des campagnes.
- `go test -count=1 ./...`, `go vet ./...`, `go test -race -count=1 ./...`, tests JS, builds Linux/Windows/macOS, CI GitHub.
- Recette Fedora : vrai `sysbench 1.0.20`, `check`, `run` combiné et un rapport contenant **5 mesures** avant de fusionner.

## Risques résiduels

1. Deux préflights Sysbench sont effectués dans une campagne combinée : coût négligeable, mais un remplacement de l'exécutable entre sous-tests n'est pas entièrement empêché. Vérification de version seulement.
2. Les tests CPU chauffent la machine avant le test mémoire : cela peut affecter les valeurs. Une campagne combinée n'est pas équivalente méthodologiquement à deux campagnes indépendantes. L'ordre est donc fixe et documenté.
3. Le JSON v1 n'identifie toujours pas les paramètres via un `protocol_id` ; l'équivalence des profils est un contrat de code, non certifiée par les données.
4. Les pourcentages sur une mesure unique ne sont pas statistiquement robustes ; répétitions et médianes restent hors scope.
5. L'avertissement chronologique est indicatif : une comparaison inversée reste possible à la demande de l'utilisateur.
6. Les mécanismes de verrou et de publication existants n'ont pas été modifiés ; ils conservent leurs propres limitations documentées en PR #4 et #5.

## Décision

**GO WITH CONDITIONS** : CI sur SHA exact et recette réelle Fedora 44 pour la campagne combinée avant tout merge.
