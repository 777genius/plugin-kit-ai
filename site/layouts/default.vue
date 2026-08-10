<template>
  <div class="app-shell">
    <a class="skip-link" href="#main-content">Skip to content</a>
    <PageBackground />
    <AppHeader />
    <main id="main-content">
      <slot />
    </main>
    <AppFooter />
  </div>
</template>
