import React, { useState } from 'react';
import { Leaf, Loader2, UserRound } from 'lucide-react';
import { apiPost, setToken } from '../api/client';
import { LoginResponse } from '../api/types';
import photo from '../assets/photo.jpg';

interface LoginViewProps {
  onSignIn: () => void;
  onGuestLogin: () => void;
}

const LoginView: React.FC<LoginViewProps> = ({ onSignIn, onGuestLogin }) => {
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
      onSignIn();
    } catch (err) {
      setError('Invalid email or password.');
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <div className="flex min-h-screen items-center justify-center bg-bg p-4 md:p-8">
      <div className="animate-in fade-in grid w-full max-w-4xl overflow-hidden rounded-card bg-surface shadow-lg duration-500 md:min-h-[560px] md:grid-cols-[1fr_1.1fr]">
        <div className="relative flex flex-col overflow-hidden bg-accent-2-200 p-9">
          <div
            aria-hidden
            className="blob-anim absolute -right-[110px] -top-[90px] h-[300px] w-[300px] bg-accent-2-300"
            style={{
              borderRadius: '47% 53% 42% 58% / 58% 50% 50% 42%',
              animation: 'blob-drift 14s ease-in-out infinite, blob-morph 11s ease-in-out infinite',
            }}
          />
          <div
            aria-hidden
            className="blob-anim absolute -bottom-[70px] -left-[90px] h-[230px] w-[230px] bg-accent-2-100 opacity-70"
            style={{
              borderRadius: '55% 45% 52% 48% / 46% 56% 44% 54%',
              animation: 'blob-drift 18s ease-in-out -6s infinite reverse, blob-morph 13s ease-in-out -3s infinite',
            }}
          />
          <div
            aria-hidden
            className="blob-anim absolute left-[8%] top-[46%] h-[120px] w-[120px] rounded-full bg-accent-200 opacity-[0.55]"
            style={{ animation: 'blob-drift 16s ease-in-out -10s infinite' }}
          />

          <div className="relative flex items-center gap-2.5">
            <div className="grid h-9 w-9 place-items-center rounded-full bg-neutral-100">
              <Leaf size={18} strokeWidth={2.75} className="text-accent-2-800" />
            </div>
            <span className="font-display text-[19px] text-accent-2-900">Nutri</span>
          </div>

          <h2 className="relative mt-7 max-w-[9ch] text-[28px] leading-[1.1] text-accent-2-900 md:text-[34px]">
            Welcome back to the table.
          </h2>
          <p className="relative mt-3 max-w-[26ch] text-sm leading-relaxed text-accent-2-800">
            Your log, your larder and Nutri are right where you left them.
          </p>

          <img
            src={photo}
            alt=""
            className="washed blob-anim relative mt-auto hidden h-[180px] w-[200px] self-end object-cover md:block"
            style={{
              borderRadius: '47% 53% 42% 58% / 58% 50% 50% 42%',
              animation: 'blob-morph 12s ease-in-out infinite',
            }}
          />
        </div>

        <form onSubmit={handleSubmit} className="flex flex-col justify-center p-8 md:p-12">
          <h3 className="mb-5 text-[25px] text-ink">Sign in</h3>

          <div className="mb-4">
            <label className="field-label" htmlFor="login-email">
              Email
            </label>
            <input
              id="login-email"
              type="email"
              autoComplete="email"
              placeholder="me@example.com"
              className="input"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              required
              disabled={isSubmitting}
            />
          </div>

          <div className="mb-2">
            <label className="field-label" htmlFor="login-password">
              Password
            </label>
            <input
              id="login-password"
              type="password"
              autoComplete="current-password"
              placeholder="••••••••"
              className="input"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              required
              disabled={isSubmitting}
            />
          </div>

          {error && (
            <p role="alert" className="mt-3 text-sm font-semibold text-accent-800">
              {error}
            </p>
          )}

          <button type="submit" disabled={isSubmitting} className="btn btn-primary mt-5 w-full py-3 text-[15px] shadow-sm flex items-center justify-center gap-2">
            {isSubmitting ? <Loader2 className="animate-spin" size={18} /> : 'Sign in'}
          </button>

          {/* Sign-up and password recovery have no endpoint behind them yet
              (the API exposes login only), so the screen says so rather than
              offering controls that lead nowhere. */}
          <p className="mt-5 text-[13px] leading-relaxed text-neutral-700">
            Accounts are created by invitation while Nutri is in testing — sign-up and password
            recovery are not open yet. Continue as a guest below to look around.
          </p>

          <div className="kicker mb-2 mt-6 text-center text-neutral-700">Guest access · testing only</div>
          <button
            type="button"
            disabled={isSubmitting}
            onClick={onGuestLogin}
            className="btn w-full border border-dashed border-neutral-500 py-2.5 text-[13px] text-neutral-700 hover:bg-neutral-200 active:bg-neutral-300"
          >
            <UserRound size={15} strokeWidth={2.5} />
            Continue as guest
          </button>
        </form>
      </div>
    </div>
  );
};

export default LoginView;
