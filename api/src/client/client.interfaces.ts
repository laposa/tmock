export interface ClientCondition {
  and?: ClientCondition[];
  or?: ClientCondition[];
  not?: boolean;
  headerMatch?: { header: string; value: string };
  headerRegex?: { header: string; value: string };
  ip?: string;
  cidr?: string;
}
