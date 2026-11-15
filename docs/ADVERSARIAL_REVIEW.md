# PC Benchmark - revue adversariale de la conception et du produit

Date : 15 novembre 2026. **Décision : GO WITH CONDITIONS** pour un MVP local en lecture seule.

## 1. État observé

- `benchmark_pc_v6.sh` est un script Bash d'orchestration exécutant Sysbench (CPU/RAM), FFmpeg (H.264 logiciel) et `benchmark_go.go` (SHA-256 parallèle).
- La machine de référence est un i3-4130 (2 cœurs / 4 threads) sur MSI B85-G43, avec environ 12 Go installés et Fedora 44.
- Le véritable rapport v5 fourni a été importé par le MVP : 8 résultats (3 CPU, 2 RAM, 3 FFmpeg), horodatage et SHA-256 vidéo.
- Le rapport v5 n'a pas de résultat Go, ce qui est normal : l'ajout Go date de v6.
- Un bug de préflight v3/v4 a montré qu'un test d'encodeur apparemment trivial pouvait provoquer un faux négatif. C'est une raison supplémentaire de séparer collecte, mesure et visualisation.

## 2. Hypothèses mises à l'épreuve

| Hypothèse | Contre-argument / mode d'échec | Décision |
|---|---|---|
| « Go remplace naturellement Bash » | La migration introduirait de nouveaux bugs sans améliorer la mesure déjà validée | Garder le script v6 inchangé ; Go sert à lire/afficher |
| « Une interface HTML rend le benchmark meilleur » | Elle peut introduire une complexité d'exécution, de sécurité et de packaging | Commencer par l'analyse, pas par le pilotage |
| « Deux temps FFmpeg suffisent à calculer un gain » | Entrées, versions ou paramètres peuvent différer | Vérifier SHA vidéo et version ; rester prudent sur les paramètres non enregistrés |
| « Une campagne représente la performance réelle » | 2 s de FFmpeg, variation de scheduling, cache, turbo, thermique | Afficher les mesures, ne pas revendiquer de précision absolue |
| « Concurrence = parallélisme » | Les goroutines ne garantissent ni scaling ni absence de contention | Le benchmark Go reste complémentaire, 1/4/8 workers |
| « Localhost est automatiquement sans risque » | Requêtes d'autres pages du navigateur, fichiers locaux, endpoints futurs | Serveur strictement lecture seule, CSP, pas de commande, bind loopback |
| « Parser du texte est une API » | Format humain variable, locales et versions susceptibles de casser l'import | Adapter isolé et limité aux marqueurs v5/v6 ; JSON versionné plus tard |

## 3. Menaces, sévérité, parades

1. **Exécution de commandes en HTTP (haute)** : elle pourrait créer une interface privilégiée et exposée à des requêtes malveillantes. **Éliminée du MVP**. Pas de sous-processus déclenché par HTTP, pas de `sudo`.
2. **Faux gains (haute)** : entrée vidéo ou outil différent. **Réduction du risque** par vérification de hash/version, fin de rapport, sens des mesures et non-comparabilité explicite. Lacune : paramètres complets du protocole v5/v6 non identifiés de façon cryptographique.
3. **Fuite de données système (moyenne)** : `hostnamectl` dans les rapports contient Machine ID et Boot ID. **Réduction** : parser à liste blanche, pas d'affichage de texte brut ni téléchargement de rapports depuis le serveur.
4. **Traversée de chemin (moyenne)** : identifiant API utilisé comme chemin disque. **Évitée** : comparaison seulement par sélection dans des IDs chargés, validation de longueur, aucun chemin transmis à `Open`.
5. **Disponibilité/mémoire (faible en local)** : rapport énorme ou beaucoup de rapports. **Bornée** : 4 Mio/fichier, 500 fichiers et lecture à la demande. À grande échelle, indexer plutôt que parser à chaque requête.
6. **Surcharge de fonctionnalités (moyenne)** : un produit de bench complet demanderait monitoring, historique, classement, ordonnanceur, profils, tests GPU/SSD. **Rejet** : seule la chaîne avant/après existante est retenue.

## 4. Produit : ce qu'il est et n'est pas

**Proposition de valeur** : mesurer et expliquer l'effet d'un changement matériel **sur la même machine**, sans perdre la traçabilité des essais. Utilisateur cible : particulier avancé / technicien Linux. Exécution locale, sans compte ni cloud.

**N'est pas** : un outil universel type Geekbench, un benchmark certifié, un portail multi-machines, un orchestrateur d'installation système ou un remplacement de la suite v6.

**Fonctionnel en v0.1** : affichage des mesures, inventaire sélectionné, comparaison, avertissements, exécutable autonome. **Absent** : pilotage, métriques thermiques, statistiques multi-runs, format JSON canonique.

## 5. Architecture retenue

```text
rapport_*.txt (source historique, inchangée)
    | lecture bornée
    v
internal/report (parser whitelist, validation, comparison)
    | structures typées
    v
Go net/http (GET seulement, loopback)
    | JSON
    v
HTML/CSS/JS embarqués (textContent, aucune dépendance externe)
```

Un seul binaire Go. Code organisé en un module de parsing/comparaison et un petit exécutable HTTP/CLI. Pas de DB, ORM, job queue, SPA framework ou généralisation spéculative.

## 6. Critères d'acceptation de cette tranche

- [x] Parse le véritable rapport v5 fourni et trouve **8 mesures**.
- [x] Affiche une campagne unique sans requérir deux rapports.
- [x] Empêche de présenter des temps FFmpeg de vidéos différentes comme comparables.
- [x] Détermine le signe du gain pour les débits **et** pour les durées.
- [x] Ne fournit aucun endpoint HTTP capable de lancer une commande.
- [x] Refuse le bind non-loopback et les identifiants de chemin arbitraires.
- [x] Tests unitaires, `go vet`, compilation binaire Linux : PASS.
- [ ] Test visuel et ergonomique sur le PC Fedora cible.
- [ ] Validation du parseur sur un vrai rapport v6 contenant les métriques Go.

## 7. Go / No-Go pour la suite

- **GO v0.1** : outil lecture seule utilisable immédiatement avec le rapport i3.
- **GATE avant v0.2** : récupérer un vrai rapport v6 et confirmer son import, puis tester deux campagnes sur le PC cible.
- **GATE avant v0.3 (pilotage)** : introduire un modèle d'état (idle/running/cancelled/failed/completed), une commande unique autorisée et des logs ; prouver qu'aucun argument utilisateur ne devient une commande shell, et qu'aucun benchmark ne peut démarrer en parallèle d'un autre.
