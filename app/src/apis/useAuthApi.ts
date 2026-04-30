export type User = {
  id: number;
  name: string;
  email: string;
  admin: boolean;
};

export const useAuthApi = () => {
  const tmockAxios = useTmockAxios();

  const login = async (email: string, password: string): Promise<{ user: User }> => {
    const response = await tmockAxios.post('/auth/login', { email, password });
    return response.data;
  };

  const logout = async () => {
    await tmockAxios.post('/auth/logout');
  };

  const getMe = async (): Promise<{ user: User }> => {
    const response = await tmockAxios.get('/auth/me');
    return response.data;
  };

  const getCsrfToken = async (): Promise<string> => {
    const response = await tmockAxios.get('/auth/csrf-token');
    return response.data.csrfToken;
  };

  return { login, logout, getMe, getCsrfToken };
};
