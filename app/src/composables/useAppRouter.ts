export const useAppRouter = () => {
  const router = useRouter();
  const route = useRoute();

  const toHome = () => router.push({ name: 'home', params: {} });

  const back = () => router.back();

  const toClients = () =>
    router.push({
      name: 'clients',
    });

  return {
    route,
    router,
    toHome,
    back,
    toClients,
  };
};
