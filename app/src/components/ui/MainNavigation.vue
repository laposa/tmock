<script setup lang="ts"></script>

<template>
  <nav>
    <RouterLink to="/" class="logo">Testing Mock Proxy</RouterLink>
    <div class="menu">
      <RouterLink to="/">Clients</RouterLink>
      <RouterLink to="/scenarios">Scenarios</RouterLink>
    </div>
  </nav>
</template>

<style scoped>
nav {
  width: 100%;
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 2rem;
  font-size: 1.2rem;
}

.logo {
  font-size: 1.5rem;
  font-weight: bold;
  text-decoration: none;
}

.menu {
  display: flex;
  gap: 20px;
}

.menu a:hover {
  filter: brightness(1.5);
  background-color: none;
}
</style>
