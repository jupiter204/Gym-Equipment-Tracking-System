import React, { useState, useEffect } from 'react';
import { useNavigate, useLocation } from 'react-router-dom';
import axios from 'axios';
import { Button } from '../../components/ui/Button';
import { Activity, AlertCircle, Loader2 } from 'lucide-react';
import apiClient, { handleForceLogout } from '../../services/apiClient';

const Login: React.FC = () => {
  const navigate = useNavigate();
  const location = useLocation();

  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [errorMessage, setErrorMessage] = useState('');
  const [isLoading, setIsLoading] = useState(false);

  // Check if redirected due to expired session
  const queryParams = new URLSearchParams(location.search);
  const isExpired = queryParams.get('expired') === 'true';

  useEffect(() => {
    handleForceLogout();
  }, []);

  const handleLogin = async (e: React.FormEvent) => {
    e.preventDefault();
    setErrorMessage('');

    if (!username.trim() || !password) {
      setErrorMessage('請輸入帳號與密碼');
      return;
    }

    setIsLoading(true);
    try {
      const response = await apiClient.post('/auth/login', {
        username: username.trim(),
        password: password,
      });

      localStorage.setItem('access_token', response.data.access_token);
      if (response.data.refresh_token) {
        localStorage.setItem('refresh_token', response.data.refresh_token);
      }

      navigate('/admin');
    } catch (err: unknown) {
      if (axios.isAxiosError(err)) {
        if (err.response?.status === 401) {
          setErrorMessage('帳號或密碼錯誤，請重新確認！');
        } else if (err.response?.status === 429) {
          setErrorMessage('登入嘗試次數過多，已被限流保護，請於數分鐘後再試。');
        } else {
          setErrorMessage(err.response?.data?.error || '伺服器錯誤，請稍後再試。');
        }
      } else {
        setErrorMessage('連線發生問題，請檢查網路狀態。');
      }
    } finally {
      setIsLoading(false);
    }
  };

  return (
    <div className="min-h-screen bg-background flex flex-col items-center justify-center p-4">
      <div className="w-full max-w-sm">
        {/* 標題與標誌區塊 */}
        <div className="flex flex-col items-center mb-8">
          <div className="w-16 h-16 bg-primary/10 rounded-full flex items-center justify-center mb-4">
            <Activity className="w-8 h-8 text-primary" />
          </div>
          <h1 className="text-3xl font-bold tracking-tight">GETS</h1>
          <p className="text-muted-foreground mt-2">運動健身器材維護系統</p>
        </div>

        {/* 登入輸入框區塊 */}
        <div className="bg-card border border-border rounded-xl shadow-lg p-6">
          {isExpired && (
            <div className="mb-4 p-3 bg-amber-500/10 border border-amber-500/20 rounded-md text-amber-500 text-sm flex items-center gap-2">
              <AlertCircle className="w-4 h-4 shrink-0" />
              <span>工作階段已過期，請重新登入。</span>
            </div>
          )}

          {errorMessage && (
            <div className="mb-4 p-3 bg-destructive/10 border border-destructive/20 rounded-md text-destructive text-sm flex items-center gap-2">
              <AlertCircle className="w-4 h-4 shrink-0" />
              <span>{errorMessage}</span>
            </div>
          )}

          <form onSubmit={handleLogin} className="space-y-4">
            <div>
              <label className="block text-sm font-medium mb-1.5">帳號</label>
              <input
                type="text"
                required
                className="w-full bg-input border border-border rounded-md px-3 py-2 focus:outline-none focus:ring-2 focus:ring-ring text-foreground"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                placeholder="請輸入帳號"
                disabled={isLoading}
              />
            </div>

            <div>
              <label className="block text-sm font-medium mb-1.5">密碼</label>
              <input
                type="password"
                required
                className="w-full bg-input border border-border rounded-md px-3 py-2 focus:outline-none focus:ring-2 focus:ring-ring text-foreground"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder="請輸入密碼"
                disabled={isLoading}
              />
            </div>

            <Button type="submit" className="w-full mt-4" disabled={isLoading}>
              {isLoading && <Loader2 className="w-4 h-4 mr-2 animate-spin" />}
              登入系統
            </Button>
          </form>
        </div>
      </div>
    </div>
  );
};

export default Login;
