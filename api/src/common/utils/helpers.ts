import { ClientCondition } from '@/client/client.interfaces';
import { IncomingMessage } from 'http';
import * as _ from 'lodash';

export const evalRequestCondition = (
  values: Record<string, any>,
  condition: string | undefined,
) => {
  if (!condition) return true;

  _.get(values, 'request.body');
  const conditionWithGet = condition.replace(
    /\$([a-zA-Z0-9_.]+)(?=[\s$])?/g,
    (match, p1) => `_.get(values, '${p1}')`,
  );

  return eval(conditionWithGet);
};

export const evalClientConditions = (
  condition: ClientCondition | null,
  req: IncomingMessage,
): boolean => {
  if (!condition) {
    return true;
  }

  const bool = condition.not ? false : true;

  if (condition.and) {
    return (
      condition.and.every((cond) => evalClientConditions(cond, req)) === bool
    );
  }

  if (condition.or) {
    return (
      condition.or.some((cond) => evalClientConditions(cond, req)) === bool
    );
  }

  if (condition.ip) {
    return condition.ip.includes(getClientIp(req)) === bool;
  }

  if (condition.cidr) {
    return isIpInCidr(getClientIp(req), condition.cidr.toString()) === bool;
  }

  if (condition.headerMatch) {
    const { header, value } = condition.headerMatch;
    return (req.headers[header.toLowerCase()] === value) === bool;
  }

  if (condition.headerRegex) {
    const { header, value: regex } = condition.headerRegex;
    const value = req.headers[header.toLowerCase()]?.toString();
    return new RegExp(regex).test(value || '') === bool;
  }

  return true;
};

export const getClientIp = (req: any) =>
  req.ip ||
  req._remoteAddress ||
  (req.connection && req.connection.remoteAddress) ||
  undefined;

function ipToBinary(ip: string): string {
  return ip
    .split('.')
    .map((num) => parseInt(num, 10).toString(2).padStart(8, '0'))
    .join('');
}

function isIpInCidr(ip: string, cidr: string): boolean {
  const [baseIp, subnetMaskLength] = cidr.split('/');
  const subnetMask = parseInt(subnetMaskLength, 10);

  const ipBinary = ipToBinary(ip);
  const baseIpBinary = ipToBinary(baseIp);

  const ipSubnet = ipBinary.substring(0, subnetMask);
  const baseIpSubnet = baseIpBinary.substring(0, subnetMask);

  return ipSubnet === baseIpSubnet;
}
