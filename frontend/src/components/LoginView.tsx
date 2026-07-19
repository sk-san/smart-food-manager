import React from 'react';
import { Code, FlaskConical, Leaf } from 'lucide-react';
import { GuestRole } from '../types/nutrition';
import photo from '../assets/photo.jpg';

interface LoginViewProps {
  /** Purely presentational — fired by the Sign in button, no real auth. */
  onSignIn: () => void;
  /** Auth bypass for testing: enters the app as the given guest role. */
  onGuestLogin: (role: GuestRole) => void;
}

// Template sign-in screen (design 2c): brand panel on the left, credential
// form on the right. There is no authentication behind it yet — the backend
// JWT flow is a follow-up — so submitting simply returns to the app.
const LoginView: React.FC<LoginViewProps> = ({ onSignIn, onGuestLogin }) => {
  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    onSignIn();
  };

  return (
    <div className="flex min-h-screen items-center justify-center bg-bg p-4 md:p-8">
      <div className="animate-in fade-in grid w-full max-w-4xl overflow-hidden rounded-card bg-surface shadow-lg duration-500 md:min-h-[560px] md:grid-cols-[1fr_1.1fr]">
        {/* Brand panel — animated blobs drift behind the copy (Sign-in Animated design) */}
        <div className="relative flex flex-col overflow-hidden bg-accent-2-200 p-9">
          <div
            aria-hidden
            className="blob-anim absolute -right-[110px] -top-[90px] h-[300px] w-[300px] bg-accent-2-300"
            style={{
              borderRadius: '47% 53% 42% 58% / 58% 50% 50% 42%',
              animation:
                'blob-drift 14s ease-in-out infinite, blob-morph 11s ease-in-out infinite',
            }}
          />
          <div
            aria-hidden
            className="blob-anim absolute -bottom-[70px] -left-[90px] h-[230px] w-[230px] bg-accent-2-100 opacity-70"
            style={{
              borderRadius: '55% 45% 52% 48% / 46% 56% 44% 54%',
              animation:
                'blob-drift 18s ease-in-out -6s infinite reverse, blob-morph 13s ease-in-out -3s infinite',
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

          {/* Washed photo in an organic frame; its silhouette morphs in step with the blobs */}
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

        {/* Sign-in form */}
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
            />
          </div>

          <a href="#" onClick={(e) => e.preventDefault()} className="self-end text-[12.5px] text-accent-700">
            Forgot password?
          </a>

          <button type="submit" className="btn btn-primary mt-5 w-full py-3 text-[15px] shadow-sm">
            Sign in
          </button>

          <div className="my-4 flex items-center gap-3 text-xs text-neutral-500">
            <span className="h-px flex-1 bg-divider" />
            new here?
            <span className="h-px flex-1 bg-divider" />
          </div>

          <button type="button" className="btn btn-secondary w-full py-3">
            Create an account
          </button>

          {/* Auth bypass for testing — no credentials, straight into the app. */}
          <div className="kicker mb-2 mt-6 text-center text-neutral-500">Guest access · testing only</div>
          <div className="flex gap-2.5">
            <button
              type="button"
              onClick={() => onGuestLogin('developer')}
              className="btn flex-1 border border-dashed border-neutral-500 py-2.5 text-[13px] text-neutral-700 hover:bg-neutral-200 active:bg-neutral-300"
            >
              <Code size={15} strokeWidth={2.5} />
              Developer
            </button>
            <button
              type="button"
              onClick={() => onGuestLogin('alpha')}
              className="btn flex-1 border border-dashed border-neutral-500 py-2.5 text-[13px] text-neutral-700 hover:bg-neutral-200 active:bg-neutral-300"
            >
              <FlaskConical size={15} strokeWidth={2.5} />
              Alpha tester
            </button>
          </div>
        </form>
      </div>
    </div>
  );
};

export default LoginView;
