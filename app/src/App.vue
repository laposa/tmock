<script setup lang="ts"></script>

<template>
  <div class="content">
    <header>
      <MainNavigation></MainNavigation>
      <SnackbarsList></SnackbarsList>
    </header>

    <main>
      <RouterView />
    </main>
  </div>
</template>

<style scoped>
.content {
  width: 100%;
  display: flex;
  flex-direction: column;
  justify-content: flex-start;
}
</style>
