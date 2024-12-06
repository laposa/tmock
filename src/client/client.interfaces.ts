export interface ClientCondition {
  and?: ClientCondition[];
  or?: ClientCondition[];
  not?: ClientCondition;
	headerMatch?: [string, string];
	headerRegex?: [string, string];
	ip?: [string];
	cidr?: [string];
}