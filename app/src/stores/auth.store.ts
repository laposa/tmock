import type { User } from '@/apis/useAuthApi';

export const useAuthStore = defineStore('auth', () => {
  const user = ref<User | null>(null);
  const csrfToken = ref<string | null>(null);
  const sessionChecked = ref(false);
  const isAuthenticated = computed(() => !!user.value);
  const isAdmin = computed(() => !!user.value?.admin);

  function setUser(data: User) {
    user.value = data;
  }

  function setCsrfToken(token: string) {
    csrfToken.value = token;
  }

  function clearAuth() {
    user.value = null;
    csrfToken.value = null;
    sessionChecked.value = false;
  }

  return { user, csrfToken, sessionChecked, isAuthenticated, isAdmin, setUser, setCsrfToken, clearAuth };
});
