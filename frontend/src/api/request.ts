import axios, { AxiosInstance, AxiosRequestConfig } from 'axios';
import { API_BASE_URL } from './runtime';

interface Result<T = unknown> {
  code: number;
  message: string;
  data: T;
}

function isObject(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null;
}

function isSuccessfulCode(code: unknown): boolean {
  return code === 0 || code === 200;
}

function isResultPayload(value: unknown): value is Result {
  if (!isObject(value)) {
    return false;
  }
  return 'code' in value && 'message' in value && 'data' in value;
}

const instance: AxiosInstance = axios.create({
  baseURL: API_BASE_URL,
  timeout: 60000,
});

instance.interceptors.response.use(
  (response) => {
    const payload = response.data as unknown;

    if (isResultPayload(payload)) {
      const result = payload;
      if (isSuccessfulCode(result.code)) {
        response.data = result.data;
        return response;
      }

      return Promise.reject(new Error(result.message || '请求失败'));
    }

    return response;
  },
  (error) => {
    if (error.response) {
      const { data } = error.response;
      if (isResultPayload(data)) {
        const result = data;
        return Promise.reject(new Error(result.message || '请求失败'));
      }

      return Promise.reject(new Error('请求失败，请重试'));
    }

    const config = error.config;
    const isUpload = config && (
      config.url?.includes('/upload') ||
      config.headers?.['Content-Type']?.toString().includes('multipart')
    );

    if (isUpload) {
      return Promise.reject(new Error('上传失败，可能是网络超时或连接中断，请重试'));
    }

    return Promise.reject(new Error('网络连接失败，请检查网络'));
  }
);

export const request = {
  get<T>(url: string, config?: AxiosRequestConfig): Promise<T> {
    return instance.get(url, config).then(res => res.data);
  },

  post<T>(url: string, data?: unknown, config?: AxiosRequestConfig): Promise<T> {
    return instance.post(url, data, config).then(res => res.data);
  },

  put<T>(url: string, data?: unknown, config?: AxiosRequestConfig): Promise<T> {
    return instance.put(url, data, config).then(res => res.data);
  },

  patch<T>(url: string, data?: unknown, config?: AxiosRequestConfig): Promise<T> {
    return instance.patch(url, data, config).then(res => res.data);
  },

  delete<T>(url: string, config?: AxiosRequestConfig): Promise<T> {
    return instance.delete(url, config).then(res => res.data);
  },

  upload<T>(url: string, formData: FormData, config?: AxiosRequestConfig): Promise<T> {
    return instance.post(url, formData, {
      timeout: 300000,
      ...config,
    }).then(res => res.data);
  },

  getInstance(): AxiosInstance {
    return instance;
  },
};

export function getErrorMessage(error: unknown): string {
  if (error instanceof Error) {
    return error.message;
  }
  return '未知错误';
}

export default request;
