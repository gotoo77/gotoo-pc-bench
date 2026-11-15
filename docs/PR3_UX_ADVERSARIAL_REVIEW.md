# PR #3 - Revue adversariale UX

## État observé (avant modification)

- Deux rapports différents portant le label `avant_upgrade` pouvaient être affichés par défaut comme un avant/après.
- Les sélecteurs identifiaient surtout le label, le CPU et l'horodatage. Deux campagnes proches restaient difficiles à distinguer.
- Une barre utilisait la valeur absolue du pourcentage de variation, ce qui affichait une régression avec une barre de progression positive.
- Les valeurs affichées provenaient directement de l'API ; les libellés `1 threads` / `1 workers` étaient incorrects au singulier.
- Les métriques n'étaient pas regroupées par CPU, RAM, FFmpeg et Go.

## Hypothèse produit

Une personne qui dispose uniquement d'un rapport avant upgrade doit voir une référence utile, sans gain fictif. Une comparaison ne doit apparaître qu'après sélection explicite d'une deuxième campagne distincte.

## Risques et contre-mesures

| Risque | Conséquence | Décision |
| --- | --- | --- |
| Deux rapports portent le même label | Comparaison prise à tort pour une amélioration matérielle | Choix explicite, identifiant de fichier visible, avertissement si labels identiques |
| Les tests en secondes sont interprétés à l'envers | Gain visuellement trompeur | Utiliser `improvement_percent` calculé côté serveur et une échelle **performance relative**, référence = 100 |
| L'échelle des barres déforme les ratios | Comparaisons exagérées | Normalisation identique pour les deux barres de **chaque métrique**, bornée par leur maximum, valeurs relatives numériques affichées |
| Métriques non comparables affichées comme gains | Conclusions erronées | Ne dessiner **aucun graphique** pour une ligne `comparable=false`, afficher la raison |
| Modification distante de `before` pendant une requête | Résultat de l'ancienne sélection affiché | Jeton d'invalidation des réponses en retard |
| Sortie des rapports injectée dans du HTML | XSS | `textContent` / `createElement`, jamais `innerHTML` |
| Un seul rapport | IHM inutile ou second champ trompeur | Affichage référence et champ « après » désactivé |
| Régression des API ou du schéma JSON | Perte de comparabilité historique | Aucun changement des packages Go, runners ou du JSON v1 |

## Périmètre implémenté

- UX : sélection explicite des deux campagnes, restauration des choix valides après actualisation.
- Référence initiale : choisir de préférence la dernière campagne marquée `avant_upgrade`, sinon la plus récente. La campagne « après » reste toujours vide tant qu'elle n'a pas été choisie.
- Intitulés de campagne comportant date, label, processeur, nombre de mesures et **nom de fichier**.
- Présentation groupée CPU / mémoire / FFmpeg / Go, légende et indicateurs normalisés (avant = 100).
- Topologie affichée comme nombre total de threads logiques seulement quand les données sont présentes.
- RAM explicitement « visible dans le rapport », sans prétendre déduire la RAM physiquement installée.
- Tests JavaScript sans dépendances tierces (`node:test`) ajoutés à la CI.

## Non objectifs

- Aucun changement dans `internal/report`, les APIs ou les scripts de benchmark.
- Aucun graphique agrégé multi-campagnes ni statistiques de médiane.
- Pas d'option pour déclencher un benchmark à partir de HTTP.
- Pas de changement de format du rapport JSON v1.

## Validation effectuée

- `go test -count=1 ./...` : PASS.
- `go vet ./...` : PASS.
- `go test -race -count=1 ./...` : PASS.
- `CGO_ENABLED=0 go build -trimpath ...` : PASS.
- `node --check` pour les deux scripts : PASS.
- `node --test tests/*.test.cjs` : 7/7 PASS.
- Navigateur Chromium headless, DOM réel avec API simulée : référence seule, comparaison explicite, changement de sélection, graphiques et absence d'erreur JS : PASS.
- Serveur Go HTTP sur localhost : 2 rapports reconnus, JS embarqué accessible, CSP maintenue : PASS.
- Capture et inspection visuelle d'une comparaison **synthétique** : PASS.

## Vérifications restantes avant fusion

- GitHub Actions pour la branche publiée.
- Vérification visuelle sur le Fedora 44 du PC cible, avec les deux rapports `avant_upgrade` réels et un second rapport distinct choisi manuellement.
- Aucune mesure i7 réelle n'a encore été observée pour cette PR ; les captures avant/après sont purement synthétiques.
