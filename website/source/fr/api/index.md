---
title: "API"
description: "Référence API générée pour plugin-kit-ai."
canonicalId: "page:api:index"
section: "api"
locale: "fr"
generated: false
translationRequired: true
aside: false
outline: false
---
<div class="docs-hero docs-hero--compact">
  <p class="docs-kicker">RÉFÉRENCE GÉNÉRÉE</p>
  <h1>API Surfaces</h1>
  <p class="docs-lead">
    Cette section rassemble les plugin-kit-ai API publics : CLI, Go SDK, les assistants d'exécution, les événements de plate-forme et les fonctionnalités.
  </p>
</div>

<div class="docs-grid">
  <a class="docs-card" href="./cli/">
    <h2>CLI</h2>
    <p>Commandes exportées depuis l'arbre Cobra vivant.</p>
  </a>
  <a class="docs-card" href="./go-sdk/">
    <h2>Go SDK</h2>
    Packages <p>Public Go pour les plugins d'exécution prêts pour la production.</p>
  </a>
  <a class="docs-card" href="./runtime-node/">
    <h2>Node Durée d'exécution</h2>
    <p>Assistants d'exécution typés pour les consommateurs JS et TS.</p>
  </a>
  <a class="docs-card" href="./runtime-python/">
    <h2>Python Durée d'exécution</h2>
    <p>Public Python assistants d'exécution uniquement, pas d'installation de wrappers.</p>
  </a>
  <a class="docs-card" href="./platform-events/">
    <h2>Événements de plateforme</h2>
    <p>Surfaces événementielles regroupées par plateforme cible.</p>
  </a>
  <a class="docs-card" href="./capabilities/">
    <h2>Capacités</h2>
    <p>Capacités regroupées sur plusieurs plates-formes et événements.</p>
  </a>
</div>

## Ouvrez la bonne surface

- Ouvrez `CLI` lorsque vous avez besoin de commandes, d'indicateurs ou du flux de travail de création.
- Ouvrez `Go SDK` lorsque vous créez un plugin d'exécution prêt pour la production dans Go.
- Ouvrez `Node Runtime` ou `Python Runtime` lorsque vous avez besoin de l'assistant partagé API pour un runtime de dépôt local.
- Ouvrez `Platform Events` lorsque vous choisissez des événements spécifiques à une cible.
- Ouvrez `Capabilities` lorsque vous souhaitez voir quelles actions et points d'extension existent sur les plates-formes.

## Ce que couvre cette section API

- l'arbre de commandes Cobra en direct
- packages publics Go
- assistants d'exécution partagés pour Node et Python
- événements spécifiques à la plateforme
- métadonnées multiplateformes au niveau des capacités