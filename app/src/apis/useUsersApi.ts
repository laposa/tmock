import type { User } from './useAuthApi';

export type CreateUserPayload = {
  name: string;
  email: string;
  password: string;
  admin: boolean;
};

export type UpdateUserPayload = {
  name?: string;
  email?: string;
  password?: string;
  admin?: boolean;
};

export type UpdateProfilePayload = {
  name?: string;
  email?: string;
  currentPassword?: string;
  newPassword?: string;
};

export const useUsersApi = () => {
  const tmockAxios = useTmockAxios();

  const getAll = async (): Promise<User[]> => {
    const response = await tmockAxios.get('/user');
    return response.data;
  };

  const create = async (data: CreateUserPayload) => {
    const response = await tmockAxios.post('/user', data);
    return response.data;
  };

  const update = async (id: number, data: UpdateUserPayload) => {
    const response = await tmockAxios.patch(`/user/${id}`, data);
    return response.data;
  };

  const remove = async (id: number) => {
    const response = await tmockAxios.delete(`/user/${id}`);
    return response.data;
  };

  const updateProfile = async (data: UpdateProfilePayload) => {
    const response = await tmockAxios.patch('/user/profile', data);
    return response.data;
  };

  return { getAll, create, update, remove, updateProfile };
};
