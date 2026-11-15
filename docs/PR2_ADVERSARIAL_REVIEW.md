# PR #2 - Adversarial review du stockage JSON v1

## Scope vérifié

- Format disque JSON v1 stable et documenté, sans changement des tests CPU/RAM/FFmpeg/Go ni de l'IHM.
- Export explicite depuis les rapports texte v5/v6, sans écrasement ni perte de l'original.
- Lecture hybride, priorité au JSON valide à nom identique, repli sur `.txt` si JSON invalide.
- Validation de forme, taille, checksum et sémantique métrique (unités / direction).

## Menaces et résultats

| Vecteur adversarial | Comportement vérifié | Résultat |
| --- | --- | --- |
| `schema_version` absente/inconnue | Rejet | PASS |
| Champ inconnu, manquant, `null` ou répété | Rejet | PASS |
| JSON incomplet, plusieurs documents, fichier > 4 MiB | Rejet | PASS |
| Métrique non positive, doublon ou unité incohérente | Rejet | PASS |
| ID interne discordant avec le nom du fichier | Rejet | PASS |
| Source symlink / traversée de chemins | Rejet par l'export | PASS |
| Export vers un fichier existant | Échec sans écraser | PASS |
| JSON corrompu voisin d'un TXT valide | TXT conservé et affiché | PASS |
| JSON et TXT valides de même base | Un seul rapport visible | PASS |
| Unité ou direction de mesure changée entre campagnes | Gain marqué non comparable | PASS |

## Limites / risques résiduels

1. **Un schéma JSON ne garantit pas l'équivalence expérimentale.** Aucun `protocol_id` explicite n'est présent dans les rapports historiques. Les comparaisons restent soumises aux garde-fous existants et seront renforcées dans une prochaine étape dédiée au moteur de benchmark.
2. **Les fichiers JSON malformés ne remontent pas directement dans la liste de l'IHM.** Si un `.txt` existe, celui-ci reste accessible ; sinon le JSON est ignoré. Un futur écran de diagnostic pourra exposer cette erreur sans l'intégrer à l'API de comparaison.
3. **L'export vérifie `Lstat` puis ouvre le fichier source** : une autre application locale pourrait effectuer une substitution entre les deux opérations. Le programme n'est pas privilégié et l'utilisateur doit contrôler son dossier de rapports.
4. **`0600` est garanti sur les Unix usuels**, pas une politique de permissions identique sous Windows. Aucune donnée n'est automatiquement envoyée sur Internet.
5. **Une seule exécution par métrique est enregistrée**. Pas de médiane ni d'incertitude statistique dans ce scope.

## Validation exécutée

- `go test -count=1 ./...` PASS
- `go vet ./...` PASS
- `go test -race -count=1 ./...` PASS
- Build `CGO_ENABLED=0` PASS
- Syntaxe schéma JSON PASS

## Décision

GO pour revue en PR. Ne pas fusionner avant CI GitHub verte. Pas de changement de dépendances ni de script de mesure, ni d'activation de commandes HTTP.
