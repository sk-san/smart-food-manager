export interface Health {
  status: string;
  db: boolean;
  time: string;
}

export interface Nutrient {
  id: number;
  code: string;
  name: string;
  unit: string;
  focus: "deficiency_watch" | "excess_watch" | "caution";
  reference_daily_amount: number | null;
}

export interface LoginResponse {
  token: string;
}
