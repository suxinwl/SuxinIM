import merge from 'lodash/merge';
import type { LanguageInterface } from './types';
type LanguageTree = { [key: string]: string | LanguageTree };
class Language implements LanguageInterface {
  private data: LanguageTree = {};
  private locale = 'zh-CN';
  constructor(locale: string, data: LanguageTree = {}) { this.locale = locale; this.data = data; }
  add(data: LanguageTree) { this.data = merge(this.data, data); }
  get<T extends string | object = Record<string, string>>(...keys: string[]): T {
    let value: string | LanguageTree = this.data[this.locale] || {};
    for (const key of keys) {
      if (typeof value !== 'object') break;
      value = value[key] || '';
    }
    return value as T;
  }
}
export default Language;
