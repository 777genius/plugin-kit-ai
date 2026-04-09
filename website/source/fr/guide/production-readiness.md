---
title: "Préparation à la production"
description: "Une liste de contrôle publique pour décider si un projet plugin-kit-ai est prêt pour l'IC, le transfert et le partage à grande échelle."
canonicalId: "page:guide:production-readiness"
section: "guide"
locale: "fr"
generated: false
translationRequired: true
---
# Préparation à la production

Utilisez cette liste de contrôle avant de qualifier un projet de prêt pour la production, prêt pour le transfert ou prêt à être diffusé à grande échelle.

<MermaidDiagram
  :chart="`
flowchart LR
  Path[Lane chosen on purpose] --> Source[Un dépôt rédigé]
  Source --> Contrôles[générer et valider les portes]
  Vérifications -> Limite [Limite de support confirmée]
  Limite -> Transfert [Les documents et le transfert sont explicites]
  Transfert -> Prêt[Production prête]
`"
/>

## 1. Choisissez volontairement le bon chemin

- par défaut Go lorsque vous voulez la voie d'exécution la plus puissante
- choisissez Node/TypeScript ou Python lorsque le compromis d'exécution locale non-Go est réel
- choisissez les voies de package, d'extension ou d'intégration uniquement lorsque ce sont les véritables résultats dont vous avez besoin

## 2. Gardez un dépôt honnête

- conserver la source du projet dans la mise en page standard du package
- traitez les fichiers cibles générés comme des sorties, et non comme l'endroit principal que vous modifiez
- ne corrigez pas les fichiers générés à la main et attendez-vous à ce que `generate` conserve ces modifications

## 3. Exécutez les portes du contrat

Au minimum, le dépôt doit survivre proprement à ce flux :

```bash
plugin-kit-ai doctor .
plugin-kit-ai generate .
plugin-kit-ai validate . --platform <target> --strict
```

Pour les voies d'exécution Python et Node, `doctor` et `bootstrap` font partie de la préparation.

## 4. Vérifiez la limite exacte du support

- confirmer que la voie principale et chaque voie supplémentaire incluse dans le champ d'application se trouvent à l'intérieur des limites du support public.
- utilisez les pages de référence lorsque vous avez besoin de termes exacts `public-stable`, `public-beta` ou `public-experimental`
- vérifiez la matrice de support cible générée avant de promettre la compatibilité aux utilisateurs en aval

## 5. Gardez l'histoire d'installation et l'histoire API séparées

- Les packages Homebrew, npm et PyPI sont des canaux d'installation pour CLI
- ce ne sont pas des surfaces d'exécution API ou SDK
- le public API réside dans la section API générée et dans les workflows documentés

## 6. Documentez le transfert

Un dépôt public devrait rendre ces choses évidentes :

- quelle voie est principale
- quelles voies supplémentaires sont réellement prises en charge
- quel moteur d'exécution il utilise et si cela change selon la cible
- quel jeu de commandes est la porte de validation canonique
- si cela dépend d'un package d'exécution partagé ou d'un chemin Go SDK

## Règle finale

Si un coéquipier ne peut pas cloner le dépôt, exécuter le flux documenté, transmettre `validate --strict` et comprendre la voie choisie sans connaissances tribales, le projet n'est pas encore prêt pour la production.