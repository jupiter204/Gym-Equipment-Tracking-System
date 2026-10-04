import axios, { type AxiosRequestConfig } from 'axios';

interface CustomAxiosRequestConfig extends AxiosRequestConfig {
  _retry?: boolean;
}

const apiClient = axios.create({
  baseURL: '/api',
  headers: {
    'Content-Type': 'application/json',
  },
});

// 1. 請求攔截器 (Request Interceptor)
apiClient.interceptors.request.use(
  (config) => {
    const token = localStorage.getItem('access_token');
    if (token) {
      config.headers.Authorization = `Bearer ${token}`;
    }
    return config;
  },
  (error) => Promise.reject(error)
);

// 2. 回應攔截器 (Response Interceptor) - 具備並行請求排隊機制的 Token 刷新
let isRefreshing = false;
let failedQueue: Array<{
  resolve: (token: string) => void;
  reject: (err: unknown) => void;
}> = [];

const processQueue = (error: unknown, token: string | null = null) => {
  failedQueue.forEach((prom) => {
    if (error) {
      prom.reject(error);
    } else if (token) {
      prom.resolve(token);
    }
  });
  failedQueue = [];
};

apiClient.interceptors.response.use(
  (response) => response,
  async (error) => {
    const originalRequest = error.config as CustomAxiosRequestConfig;

    if (!originalRequest) {
      return Promise.reject(error);
    }

    // 排除登入與刷新認證端點，避免錯誤重試迴圈卡死
    const url = originalRequest.url ?? '';
    if (url.includes('auth/login') || url.includes('auth/refresh')) {
      return Promise.reject(error);
    }

    // 當後端回傳 401 Unauthorized 且尚未重試過
    if (error.response?.status === 401 && !originalRequest._retry) {
      if (isRefreshing) {
        // 如果已經有其他請求正在刷新 Token，則加入排隊隊列 (附帶 10 秒安全超時防護避免掛起)
        return new Promise<string>((resolve, reject) => {
          const timer = setTimeout(() => {
            reject(new Error('Token refresh timeout'));
          }, 10000);

          failedQueue.push({
            resolve: (token: string) => {
              clearTimeout(timer);
              resolve(token);
            },
            reject: (err: unknown) => {
              clearTimeout(timer);
              reject(err);
            },
          });
        })
          .then((token) => {
            if (originalRequest.headers) {
              originalRequest.headers.Authorization = `Bearer ${token}`;
            }
            return apiClient(originalRequest);
          })
          .catch((err) => Promise.reject(err));
      }

      originalRequest._retry = true;
      isRefreshing = true;

      const currentRefreshToken = localStorage.getItem('refresh_token');

      try {
        if (!currentRefreshToken) {
          processQueue(error, null);
          handleForceLogout();
          return Promise.reject(error);
        }

        const res = await axios.post('/api/auth/refresh', { refresh_token: currentRefreshToken });
        const { access_token, refresh_token } = res.data || {};

        if (access_token) {
          localStorage.setItem('access_token', access_token);
          if (refresh_token) {
            localStorage.setItem('refresh_token', refresh_token);
          }

          apiClient.defaults.headers.common['Authorization'] = `Bearer ${access_token}`;
          if (originalRequest.headers) {
            originalRequest.headers.Authorization = `Bearer ${access_token}`;
          }

          processQueue(null, access_token);
          return apiClient(originalRequest);
        } else {
          throw new Error('Refresh response missing access_token');
        }
      } catch (refreshError) {
        // 多標籤頁判斷：若其他分頁在此期間已成功刷新，則直接使用最新 access_token 重試
        const latestRefreshToken = localStorage.getItem('refresh_token');
        const latestAccessToken = localStorage.getItem('access_token');
        if (latestRefreshToken && latestRefreshToken !== currentRefreshToken && latestAccessToken) {
          apiClient.defaults.headers.common['Authorization'] = `Bearer ${latestAccessToken}`;
          if (originalRequest.headers) {
            originalRequest.headers.Authorization = `Bearer ${latestAccessToken}`;
          }
          processQueue(null, latestAccessToken);
          return apiClient(originalRequest);
        }

        processQueue(refreshError, null);
        handleForceLogout();
        return Promise.reject(refreshError);
      } finally {
        isRefreshing = false;
      }
    }

    return Promise.reject(error);
  }
);

// 重置攔截器刷新狀態 (不清除 Token)
export function resetRefreshState() {
  isRefreshing = false;
  failedQueue = [];
}

// 強制登出輔助函式
export function handleForceLogout() {
  resetRefreshState();
  localStorage.removeItem('access_token');
  localStorage.removeItem('refresh_token');
  if (apiClient.defaults.headers.common['Authorization']) {
    delete apiClient.defaults.headers.common['Authorization'];
  }
  if (window.location.pathname.startsWith('/admin')) {
    window.location.href = '/login?expired=true';
  }
}

// 解析 Access Token 取得使用者資訊 (支援 base64url 與 UTF-8 字元)
export function getStoredUser(): { userUUID: string; role: 'admin' | 'staff' } | null {
  const token = localStorage.getItem('access_token');
  if (!token) return null;
  try {
    const parts = token.split('.');
    if (parts.length !== 3) return null;
    let base64 = parts[1].replace(/-/g, '+').replace(/_/g, '/');
    while (base64.length % 4 !== 0) {
      base64 += '=';
    }
    const binaryStr = atob(base64);
    const bytes = Uint8Array.from(binaryStr, (c) => c.charCodeAt(0));
    const jsonStr = new TextDecoder().decode(bytes);
    const payload = JSON.parse(jsonStr);
    return {
      userUUID: payload.userUUID,
      role: payload.role,
    };
  } catch {
    return null;
  }
}

export default apiClient;