export function omitPassword<T extends { password: string }>(
  user: T,
): Omit<T, 'password'> {
  const { password: _, ...rest } = user;
  return rest;
}
