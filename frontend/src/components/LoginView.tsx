import React, { useState } from 'react';
import { Utensils, Loader2 } from 'lucide-react';
import { apiPost, setToken } from '../api/client';
import { LoginResponse } from '../api/types';

interface LoginViewProps {
  onLoggedIn: () => void;
}

const LoginView: React.FC<LoginViewProps> = ({ onLoggedIn }) => {
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!email || !password || isSubmitting) return;

    setIsSubmitting(true);
    setError(null);
    try {
      const data = await apiPost<LoginResponse>('/api/v1/auth/login', { email, password });
      setToken(data.token);
      onLoggedIn();
    } catch (err) {
      setError('Invalid email or password.');
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <div className="min-h-screen bg-surface flex items-center justify-center p-4">
      <div className="w-full max-w-sm bg-surface-container-lowest rounded-[28px] p-8 border border-outline-variant shadow-sm">
        <div className="flex flex-col items-center mb-8">
          <div className="w-14 h-14 bg-primary-container rounded-2xl flex items-center justify-center mb-4">
            <Utensils className="text-on-primary-container" size={28} />
          </div>
          <h1 className="text-headline-sm font-normal text-on-surface">Welcome back</h1>
          <p className="text-on-surface-variant text-body-md mt-1">Log in to NutriMind</p>
        </div>

        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label className="block text-label-lg font-medium text-on-surface-variant mb-1">Email</label>
            <input
              type="email"
              autoComplete="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              className="w-full bg-surface-container border-b-2 border-outline hover:border-on-surface-variant focus:border-primary rounded-t-lg px-4 py-3 text-on-surface outline-none transition-colors"
              placeholder="you@example.com"
              required
            />
          </div>
          <div>
            <label className="block text-label-lg font-medium text-on-surface-variant mb-1">Password</label>
            <input
              type="password"
              autoComplete="current-password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              className="w-full bg-surface-container border-b-2 border-outline hover:border-on-surface-variant focus:border-primary rounded-t-lg px-4 py-3 text-on-surface outline-none transition-colors"
              placeholder="••••••••"
              required
            />
          </div>

          {error && <p className="text-red-600 text-body-md">{error}</p>}

          <button
            type="submit"
            disabled={isSubmitting}
            className="w-full py-3 mt-2 bg-primary text-on-primary rounded-full font-medium shadow-md hover:shadow-lg disabled:opacity-50 disabled:cursor-not-allowed flex items-center justify-center gap-2 transition-all"
          >
            {isSubmitting ? <Loader2 className="animate-spin" size={20} /> : 'Log In'}
          </button>
        </form>
      </div>
    </div>
  );
};

export default LoginView;
