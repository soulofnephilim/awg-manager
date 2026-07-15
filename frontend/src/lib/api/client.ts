// Фасад API-клиента. Доменные методы разнесены по слоям client*.ts
// (цепочка наследования от CoreClient); публичная поверхность не менялась:
// `api` и сопутствующие экспорты доступны по прежнему пути $lib/api/client.
import { FreeturnClient } from './clientFreeturn';

export { ApiGatewayError } from './clientCore';
export type { TrafficPeriod } from './clientCore';

class ApiClient extends FreeturnClient {}

export const api = new ApiClient();
