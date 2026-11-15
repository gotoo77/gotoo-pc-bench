# PR #7 - FFmpeg runner Go - Revue adversariale

## Périmètre

- Runner **Linux/CLI seulement** avec `ffmpeg`, `ffprobe` et `libx264` existants.
- Sélection `--test ffmpeg`, `cpu,ffmpeg`, `memory,ffmpeg`, `cpu,memory,ffmpeg` ; ordre stable CPU, RAM, FFmpeg.
- `--video` requis exclusivement si FFmpeg est sélectionné ; source **existante**, jamais générée, téléchargée, réencodée en fichier ou supprimée.
- Protocole vérifié dans `benchmark_pc_v6.sh` et la sortie historique i3 du 15 novembre 2026 (FFV1 640×360, 30 fps, yuv420p, 20 s, SHA-256 `ea7ca7ecd8795138031a703e324952653277928bea91208058dea7a6005c44d2`).
- **Aucun changement** au format JSON v1, au parser historique, au comparateur, au serveur HTTP et au frontend.

## Invariants et validations

| Exigence | Vérification |
| --- | --- |
| Préflight avant toute charge de la campagne | `CheckSelected` avant `RunCPU` / `RunMemory`, faux FFmpeg cassé, zéro appel CPU |
| Codec réellement utilisable | `ffmpeg -encoders` + encodage d'une image de test `testsrc2` vers `null` |
| Vidéo source conforme | Lstat régulier, sans lien symbolique, 1..512 Mio, FFprobe FFV1 640×360 yuv420p 30/1 20.000000 s |
| Un seul protocole FFmpeg | Paramètres historiques exacts, `-threads` 1/4/8, `libx264 medium CRF 23`, sans fichier de sortie |
| Charge réellement effectuée | FFmpeg code retour 0 et dernière annonce `frame=600` pour chaque encodage |
| Mesure fidèle | Temps réel monotone entourant le processus, arrondi à 3 décimales (`RESULT ... elapsed_seconds` historique) |
| Source identique pour les 3 mesures | SHA-256 au départ et après les encodages, échec si modifiée |
| Publication atomique | `WriteResult` appelé une seule fois après la réussite de tous les modules |
| Respect d'annulation | Contexte partagé et sous-processus FFmpeg/FFprobe avec timeout, aucun rapport partiel retourné |
| Confidentialité | Rapport contient SHA, codec/dimensions/durée, version FFmpeg, mesures ; jamais le chemin source |
| Rétrocompatibilité | Lecture JSON v1 et `.txt` inchangée ; frontend regroupe déjà les métriques `ffmpeg.*` |
| Portabilité de consultation | Compilation croisée Linux/Windows/macOS ; seules les charges sont Linux-only |

## Tests exécutés

- `go test -count=1 ./...` : PASS.
- `go vet ./...` : PASS.
- `go test -race -count=1 ./...` : PASS.
- `node --test tests/*.test.cjs` : 8/8 PASS.
- Builds Linux amd64, Windows amd64, macOS arm64 : PASS.
- Mocks CLI/end-to-end : PASS, JSON v1 à huit métriques pour CPU+RAM+FFmpeg, exactitude des arguments, refus d'une vidéo manquante/symlink/mauvais format, absence de résultat après échec ou résultat 599 frames, non-divulgation du chemin.
- **Vrai FFmpeg 7.1.5, conteneur Linux** : testsrc2 FFV1 synthétique 640×360 30 fps 20 s ; préflight PASS, trois encodages H.264 PASS, JSON 3 métriques relu par `list`. Source synthétique SHA `d87ad6cfc50ac85559f52550193ed0948e1b67b377b1e8f75d06884b54434e1f` **différent de la référence du PC cible**.
- CI GitHub : à vérifier à l'ouverture de la PR.
- **Recette sur le véritable fichier Fedora 44** : PENDING ; ne pas fusionner avant cette validation.

## Limites non résolues, acceptées pour cette tranche

1. **Format de résultat != identité du protocole** : le schéma v1 n'a pas de `protocol_id` et ne conserve pas les paramètres libx264 ni le build exact de x264. Même FFmpeg version et même SHA assurent une comparabilité pratique mais pas parfaite. Future évolution du schéma justifiable, pas dans la PR #7.
2. **Fichier source modifié pendant un sous-test** : les SHA avant/après détectent une modification persistante, mais pas un remplacement temporaire suivi d'un retour au contenu original. Le fichier et son dossier doivent rester sous le contrôle de l'utilisateur pendant les mesures. `Lstat`/`Open` ne constituent pas une sandbox parfaite.
3. **Même machine, même mesure, variations** : cache, température, fréquence CPU et processus tiers peuvent affecter les temps. Les médianes de trois campagnes restent à développer.
4. **Logs de processus** : la sortie FFmpeg est collectée en mémoire pour vérifier les images ; elle n'est ni exportée par HTTP ni persistée, mais une application FFmpeg défaillante peut produire un volume de sortie excessif. Ce runner n'est pas conçu pour exécuter un binaire FFmpeg non fiable dans le PATH.
5. **Durées fixes** : la limite de 120 s par sous-test couvre largement la référence actuelle, mais peut exclure une machine extrêmement lente. Aucune option de réglage libre n'est ajoutée à ce premier protocole pour conserver sa reproductibilité.
6. **Préflight nécessaire mais pas suffisant** : la présence de libx264 et le mini-encodage ne garantissent pas qu'une charge de 600 images terminera sans erreur ; le run vérifie explicitement chaque retour et le nombre d'images.
7. **Métriques CPU+RAM+FFmpeg seules** : la campagne complète reproduit 8 des 11 mesures historiques. Le programme Go SHA-256 historique reste hors scope, et aucune mesure n'est inventée.

## Décision

**GO WITH CONDITIONS** : intégrer en PR draft, valider CI sur le SHA exact, puis test Fedora 44 sur `benchmark_source_ffv1.mkv` historique et validation du SHA. La fusion reste conditionnée à ce dernier essai réel.
