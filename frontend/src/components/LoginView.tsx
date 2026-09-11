import React, { useState } from 'react';
import { Leaf, Loader2, UserRound, ArrowLeft, CheckCircle2 } from 'lucide-react';
import { apiPost, setToken } from '../api/client';
import { LoginResponse } from '../api/types';
import photo from '../assets/photo.jpg';

interface LoginViewProps {
  onSignIn: () => void;
  onGuestLogin: () => void;
}

const LoginView: React.FC<LoginViewProps> = ({ onSignIn, onGuestLogin }) => {
  const [mode, setMode] = useState<'signin' | 'signup' | 'forgot' | 'reset'>('signin'); 
  const [displayName, setDisplayName] = useState('');              
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [resetToken, setResetToken] = useState('');
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [successMessage, setSuccessMessage] = useState<string | null>(null);
  React.useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    const token = params.get('token');
    if (token) {
      setResetToken(token);
      setMode('reset');
    }
  }, []);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (isSubmitting) return;

    setIsSubmitting(true);
    setError(null);
    setSuccessMessage(null);
    try {
      if (mode === 'signin' || mode === 'signup') {
        const endpoint = mode === 'signin' ? '/api/v1/auth/login' : '/api/v1/auth/register';
        const payload =
          mode === 'signin'
            ? { email, password }
            : { email, password, display_name: displayName.trim() || undefined };

        const data = await apiPost<LoginResponse>(endpoint, payload);
        setToken(data.token);
        onSignIn();
      } else if (mode === 'forgot') {
        const data = await apiPost<{ message: string }>('/api/v1/auth/forgot-password', { email });
        setSuccessMessage(data.message);
      } else if (mode === 'reset') {
        const data = await apiPost<{ message: string }>('/api/v1/auth/reset-password', {
          token: resetToken,
          password,
        });
        setSuccessMessage(data.message);
        setTimeout(() => {
          setMode('signin');
          setSuccessMessage(null);
          setPassword('');
        }, 4000);
      }
    } catch (err: any) {
      if (mode === 'signup') {
        setError(err?.message || 'Could not create account.');
      } else if (mode === 'signin') {
        setError('Invalid email or password.');
      } else {
        setError(err?.message || 'An error occurred.');
      }
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

        <form onSubmit={handleSubmit} className="flex flex-col justify-start pt-12 p-8 md:p-12">
          {mode === 'forgot' || mode === 'reset' ? (
            <button
              type="button"
              onClick={() => { setMode('signin'); setError(null); setSuccessMessage(null); }}
              className="mb-6 flex items-center gap-1.5 text-sm font-semibold text-neutral-600 hover:text-ink"
            >
              <ArrowLeft size={16} />
              Back to Sign in
            </button>
          ) : (
            <div className="mb-6 flex gap-3 border-b border-neutral-200 pb-2">
              <button
                type="button"
                onClick={() => { setMode('signin'); setError(null); setSuccessMessage(null); }}
                className={`pb-1 text-[17px] font-semibold transition-colors ${
                  mode === 'signin'
                    ? 'border-b-2 border-accent-800 text-ink'
                    : 'text-neutral-500 hover:text-ink'
                }`}
              >
                Sign in
              </button>
              <button
                type="button"
                onClick={() => { setMode('signup'); setError(null); setSuccessMessage(null); }}
                className={`pb-1 text-[17px] font-semibold transition-colors ${
                  mode === 'signup'
                    ? 'border-b-2 border-accent-800 text-ink'
                    : 'text-neutral-500 hover:text-ink'
                }`}
              >
                Create account
              </button>
            </div>
          )}
          {mode === 'signup' && (
            <div className="mb-4">
              <label className="field-label" htmlFor="register-name">
                Display name (optional)
              </label>
              <input
                id="register-name"
                type="text"
                autoComplete="name"
                placeholder="Alex"
                className="input"
                value={displayName}
                onChange={(e) => setDisplayName(e.target.value)}
                disabled={isSubmitting}
                maxLength={60}
              />
            </div>
          )}

          {mode === 'reset' ? (
            <div className="mb-4">
              <label className="field-label" htmlFor="reset-token">
                Reset Token
              </label>
              <input
                id="reset-token"
                type="text"
                placeholder="Paste your reset token"
                className="input"
                value={resetToken}
                onChange={(e) => setResetToken(e.target.value)}
                required
                disabled={isSubmitting}
              />
            </div>
          ) : (
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
          )}

          {mode !== 'forgot' && (
            <div className="mb-2">
              <div className="flex items-center justify-between">
                <label className="field-label" htmlFor="login-password">
                  {mode === 'reset' ? 'New Password' : 'Password'}
                </label>
                {mode === 'signin' && (
                  <button
                    type="button"
                    onClick={() => { setMode('forgot'); setError(null); }}
                    className="text-xs text-neutral-600 underline hover:text-ink"
                  >
                    Forgot password?
                  </button>
                )}
              </div>
              <input
                id="login-password"
                type="password"
                autoComplete={mode === 'signin' ? 'current-password' : 'new-password'}
                placeholder="••••••••"
                className="input"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                required
                minLength={8}
                maxLength={72}
                disabled={isSubmitting}
              />
            </div>
          )}

          {successMessage && (
            <div className="mt-3 flex items-start gap-2 rounded-lg bg-accent-2-100 p-3 text-sm text-accent-2-900">
              <CheckCircle2 size={18} className="mt-0.5 shrink-0 text-accent-2-800" />
              <div>
                <p>{successMessage}</p>
                {mode === 'forgot' && (
                  <button
                    type="button"
                    onClick={() => { setMode('reset'); setError(null); }}
                    className="mt-1.5 block text-xs font-semibold text-accent-800 underline hover:text-accent-900"
                  >
                    Have a reset token? Enter it here →
                  </button>
                )}
              </div>
            </div>
          )}

          {error && (
            <p role="alert" className="mt-3 text-sm font-semibold text-accent-800">
              {error}
            </p>
          )}

          <button type="submit" disabled={isSubmitting} className="btn btn-primary mt-5 w-full py-3 text-[15px] shadow-sm flex items-center justify-center gap-2">
            {isSubmitting ? (
              <Loader2 className="animate-spin" size={18} />
            ) : mode === 'signin' ? (
              'Sign in'
            ) : mode === 'signup' ? (
              'Create account'
            ) : mode === 'forgot' ? (
              'Send recovery link'
            ) : (
              'Reset password'
            )}
          </button>

          {/* Sign-up and password recovery have no endpoint behind them yet
              (the API exposes login only), so the screen says so rather than
              offering controls that lead nowhere. */}
          

            {import.meta.env.DEV && (
            <>
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
            </>
          )}
        </form>
      </div>
    </div>
  );
};

export default LoginView;
