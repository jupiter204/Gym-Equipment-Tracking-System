import React, { useEffect, useState } from 'react';
import { useParams } from 'react-router-dom';
import axios from 'axios';
import apiClient from '../../services/apiClient';
import { Button } from '../../components/ui/Button';
import { AlertTriangle, CheckCircle2, Loader2 } from 'lucide-react';
import type { Equipment } from '../../types';

const ReportEquipment: React.FC = () => {
  const { id } = useParams<{ id: string }>();

  const [equipment, setEquipment] = useState<Equipment | null>(null);
  const [loading, setLoading] = useState(true);
  const [description, setDescription] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [submitted, setSubmitted] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    const fetchEquipment = async () => {
      if (id) {
        try {
          const res = await apiClient.get('/public/equipment', {
            params: { asset_code: id },
          });
          setEquipment(res.data);
        } catch {
          setEquipment(null);
        } finally {
          setLoading(false);
        }
      }
    };
    fetchEquipment();
  }, [id]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');

    if (!equipment || !description.trim()) {
      setError('請輸入故障狀況描述');
      return;
    }

    setSubmitting(true);
    try {
      await apiClient.post('/public/report', {
        equipment_id: equipment.lid,
        description: description.trim(),
      });
      setSubmitted(true);
    } catch (err: unknown) {
      if (axios.isAxiosError(err)) {
        if (err.response?.status === 409) {
          setError('此設備已有尚未處理的報修紀錄，維修專員已在排程處理中。');
        } else if (err.response?.status === 429) {
          setError('通報頻率過高，已被暫時限制，請稍候再試。');
        } else if (err.response?.status === 400) {
          setError(err.response?.data?.error || '請求格式錯誤，描述請限制於 1 到 500 字元。');
        } else {
          setError(err.response?.data?.error || '報修失敗，請稍候再試。');
        }
      } else {
        setError('無法連線至伺服器，請檢查網路連線。');
      }
    } finally {
      setSubmitting(false);
    }
  };

  if (loading) {
    return (
      <div className="min-h-screen bg-background flex items-center justify-center">
        <Loader2 className="w-8 h-8 animate-spin text-primary" />
      </div>
    );
  }

  if (!equipment) {
    return (
      <div className="min-h-screen bg-background flex flex-col items-center justify-center p-6 text-center">
        <AlertTriangle className="w-16 h-16 text-destructive mb-4" />
        <h1 className="text-2xl font-bold mb-2">找不到器材</h1>
        <p className="text-muted-foreground">請確認您掃描的 QR Code 或資產編號是否正確。</p>
      </div>
    );
  }

  if (equipment.has_active_report || submitted) {
    return (
      <div className="min-h-screen bg-background flex flex-col items-center justify-center p-6 text-center">
        <CheckCircle2 className="w-16 h-16 text-green-500 mb-4" />
        <h1 className="text-2xl font-bold mb-2">已收到通報</h1>
        <p className="text-muted-foreground text-lg">
          {submitted ? '感謝您的協助，我們會盡快處理！' : '此器材已通報故障，維修人員處理中，感謝您的提醒！'}
        </p>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-background p-6 flex flex-col max-w-md mx-auto">
      <div className="flex-1">
        <h1 className="text-2xl font-bold text-foreground mb-6">回報器材故障</h1>

        <div className="bg-card border border-border p-4 rounded-xl mb-6 shadow-sm">
          <h2 className="text-lg font-semibold mb-1 text-primary">{equipment.name}</h2>
          <p className="text-sm text-muted-foreground mb-1">編號: {equipment.asset_code}</p>
          {equipment.location && <p className="text-sm text-muted-foreground">位置: {equipment.location}</p>}
        </div>

        <form onSubmit={handleSubmit} className="space-y-4 flex flex-col">
          <div>
            <label className="block text-sm font-medium mb-2 text-foreground">故障狀況描述</label>
            <textarea
              required
              rows={4}
              maxLength={500}
              className="w-full bg-input border border-border rounded-lg p-3 text-foreground focus:ring-2 focus:ring-ring focus:outline-none resize-none transition-shadow"
              placeholder="請簡述您遇到的問題 (例如：螢幕無畫面、異音...)"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
            />
            <p className="text-xs text-muted-foreground mt-1 text-right">{description.length}/500 字</p>
          </div>

          {error && <p className="text-destructive text-sm bg-destructive/10 p-2.5 rounded-md">{error}</p>}

          <Button 
            type="submit" 
            size="lg" 
            className="w-full text-lg mt-4 h-12"
            disabled={submitting || !description.trim()}
          >
            {submitting ? <Loader2 className="w-5 h-5 animate-spin mr-2" /> : null}
            提交通報
          </Button>
        </form>
      </div>
    </div>
  );
};

export default ReportEquipment;
