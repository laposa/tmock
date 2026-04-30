<script setup lang="ts">
const authApi = useAuthApi();
const authStore = useAuthStore();
const router = useRouter();

const email = ref('');
const password = ref('');
const isLoading = ref(false);
const error = ref('');

async function handleLogin() {
  error.value = '';
  isLoading.value = true;

  try {
    const data = await authApi.login(email.value, password.value);
    authStore.setUser(data.user);
    const csrfToken = await authApi.getCsrfToken();
    authStore.setCsrfToken(csrfToken);
    router.push('/');
  } catch (err: any) {
    error.value = err.response?.data?.message || 'Login failed';
  } finally {
    isLoading.value = false;
  }
}
</script>

<template>
  <div class="login-container">
    <div class="login-card">
      <div class="login-header">
        <TmockLogoSvg />
        <h1>Tmock</h1>
      </div>

      <v-alert v-if="error" type="error" class="mb-4" density="compact">
        {{ error }}
      </v-alert>

      <form @submit.prevent="handleLogin">
        <v-text-field
          v-model="email"
          label="Email"
          type="email"
          required
          variant="outlined"
          density="comfortable"
          class="mb-2"
        />
        <v-text-field
          v-model="password"
          label="Password"
          type="password"
          required
          variant="outlined"
          density="comfortable"
          class="mb-4"
        />
        <v-btn
          type="submit"
          color="indigo"
          block
          size="large"
          :disabled="isLoading || !email || !password"
          :loading="isLoading"
        >
          Login
        </v-btn>
      </form>
    </div>
  </div>
</template>

<style scoped>
.login-container {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 100vh;
  width: 100%;
}

.login-card {
  width: 100%;
  max-width: 400px;
  padding: 2rem;
}

.login-header {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 0.5rem;
  margin-bottom: 2rem;
}

.login-header svg {
  width: 50px;
  height: 50px;
}

.login-header h1 {
  font-size: 2rem;
  font-weight: 500;
  letter-spacing: 1px;
}
</style>
