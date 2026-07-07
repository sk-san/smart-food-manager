import React, { useState, useEffect, useRef } from 'react';
import { getCompanionMessage } from '../services/nutritionService';
import { DailyGoal, NutritionData } from '../types/nutrition';

interface CompanionCharacterProps {
  stats: NutritionData;
  goals: DailyGoal;
  isLookingAtScreen?: boolean;
}

interface Particle {
  id: number;
  type: 'heart' | 'star' | 'note';
  x: number;
  y: number;
  scale: number;
}

// Mutable drag state. Lives in a ref so the per-frame physics loop can read and
// write it without triggering React re-renders.
interface DragState {
  active: boolean;
  pointerId: number | null;
  startX: number;
  startY: number;
  lastX: number;
  lastY: number;
  scaleX: number;
  scaleY: number;
  skew: number;
  tilt: number;
  frame: number;
  moved: boolean;
  pendingPoint: { clientX: number; clientY: number } | null;
  rect: DOMRect | null;
}

function clamp(value: number, min: number, max: number) {
  return Math.min(Math.max(value, min), max);
}

// "Nutri" — a soft, squishy companion you can grab and stretch. Ported from the
// standalone soft-object prototype into React: the blob is a CSS shape, the face
// and sprout are SVG, and the squish is driven by CSS custom properties updated
// imperatively inside a requestAnimationFrame loop (so dragging never re-renders).
//
// App integration is preserved from the original companion: it shows a speech
// bubble fed by getCompanionMessage, reacts when the user logs food, sprinkles
// particles, and turns away when the user is busy reading content.
const CompanionCharacter: React.FC<CompanionCharacterProps> = ({ stats, goals, isLookingAtScreen = false }) => {
  const [message, setMessage] = useState<string>("Hi! I'm Nutri! (◕‿◕)");
  const [isTyping, setIsTyping] = useState(false);
  const [isDragging, setIsDragging] = useState(false);
  const [particles, setParticles] = useState<Particle[]>([]);

  const blobRef = useRef<HTMLButtonElement>(null);
  const softRef = useRef<HTMLSpanElement>(null);
  const dragRef = useRef<DragState>({
    active: false,
    pointerId: null,
    startX: 0,
    startY: 0,
    lastX: 0,
    lastY: 0,
    scaleX: 1,
    scaleY: 1,
    skew: 0,
    tilt: 0,
    frame: 0,
    moved: false,
    pendingPoint: null,
    rect: null,
  });
  const endDragRef = useRef<() => void>(() => {});
  const reduceMotionRef = useRef(false);
  const pendingMsgRef = useRef(false);
  const mountedRef = useRef(true);
  const timeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const particleIdCounter = useRef(0);

  useEffect(() => {
    mountedRef.current = true;
    reduceMotionRef.current = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
    return () => {
      mountedRef.current = false;
      if (dragRef.current.frame) cancelAnimationFrame(dragRef.current.frame);
      if (timeoutRef.current) clearTimeout(timeoutRef.current);
    };
  }, []);

  const addParticles = (count: number, type: Particle['type']) => {
    const newParticles: Particle[] = [];
    for (let i = 0; i < count; i++) {
      newParticles.push({
        id: particleIdCounter.current++,
        type,
        x: 20 + Math.random() * 60, // Random X pos between 20% and 80%
        y: 20 + Math.random() * 30, // Random Y pos near top of character
        scale: 0.5 + Math.random() * 0.5,
      });
    }

    setParticles(prev => [...prev, ...newParticles]);

    // Cleanup particles after animation
    setTimeout(() => {
      if (!mountedRef.current) return;
      setParticles(prev => prev.filter(p => p.id >= particleIdCounter.current - count - 10)); // Rough cleanup
    }, 2000);
  };

  // --- Squish primitives ---------------------------------------------------
  // Push the current softness onto the DOM via CSS custom properties. We write
  // straight to the element (never through React state) so the rAF loop can run
  // at full frame rate without reconciliation churn.
  const updateSoftness = (scaleX: number, scaleY: number, skew: number, tilt: number) => {
    const soft = softRef.current;
    const blob = blobRef.current;
    if (soft) {
      soft.style.setProperty('--scale-x', scaleX.toFixed(3));
      soft.style.setProperty('--scale-y', scaleY.toFixed(3));
      soft.style.setProperty('--skew', `${skew.toFixed(2)}deg`);
    }
    if (blob) blob.style.setProperty('--tilt', `${tilt.toFixed(2)}deg`);
  };

  // Map the pointer into the soft object's local 0..100% space so the gloss
  // highlight and the touch ring track the finger, and the squish origin
  // anchors opposite the grab point.
  const setTouchPoint = (clientX: number, clientY: number, setAnchor = false) => {
    const soft = softRef.current;
    if (!soft) return;
    const rect = dragRef.current.rect || soft.getBoundingClientRect();
    const x = clamp(((clientX - rect.left) / rect.width) * 100, 0, 100);
    const y = clamp(((clientY - rect.top) / rect.height) * 100, 0, 100);

    soft.style.setProperty('--touch-x', `${x}%`);
    soft.style.setProperty('--touch-y', `${y}%`);

    if (setAnchor) {
      soft.style.setProperty('--origin-x', `${100 - x}%`);
      soft.style.setProperty('--origin-y', `${100 - y}%`);
    }
  };

  const renderDrag = () => {
    const drag = dragRef.current;
    if (!drag.active || !drag.pendingPoint) return;

    const { clientX, clientY } = drag.pendingPoint;
    const dx = clientX - drag.startX;
    const dy = clientY - drag.startY;
    const speedX = clientX - drag.lastX;
    const speedY = clientY - drag.lastY;
    const pull = Math.hypot(dx, dy);
    const stretch = clamp(pull / 170, 0, 0.34);
    const squeeze = clamp(pull / 260, 0, 0.18);
    const velocity = clamp(Math.hypot(speedX, speedY) / 42, 0, 0.22);
    const targetScaleX = 1 + stretch + velocity;
    const targetScaleY = 1 - squeeze;
    const targetSkew = clamp(dx / 20, -10, 10);
    const targetTilt = clamp(dx / 24 + dy / 44, -13, 13);

    drag.scaleX += (targetScaleX - drag.scaleX) * 0.22;
    drag.scaleY += (targetScaleY - drag.scaleY) * 0.22;
    drag.skew += (targetSkew - drag.skew) * 0.2;
    drag.tilt += (targetTilt - drag.tilt) * 0.18;

    updateSoftness(drag.scaleX, drag.scaleY, drag.skew, drag.tilt);
    setTouchPoint(clientX, clientY);

    if (pull > 6) drag.moved = true;
    drag.lastX = clientX;
    drag.lastY = clientY;

    drag.frame = requestAnimationFrame(renderDrag);
  };

  const startDrag = (e: React.PointerEvent<HTMLButtonElement>) => {
    if (isLookingAtScreen) return; // turned away — not in the mood to be grabbed
    if (e.button !== 0 && e.pointerType === 'mouse') return;

    const drag = dragRef.current;
    const soft = softRef.current;
    drag.active = true;
    drag.pointerId = e.pointerId;
    drag.startX = e.clientX;
    drag.startY = e.clientY;
    drag.lastX = e.clientX;
    drag.lastY = e.clientY;
    drag.scaleX = 1;
    drag.scaleY = 1;
    drag.skew = 0;
    drag.tilt = 0;
    drag.moved = false;
    drag.pendingPoint = { clientX: e.clientX, clientY: e.clientY };
    drag.rect = soft ? soft.getBoundingClientRect() : null;

    try {
      e.currentTarget.setPointerCapture(e.pointerId);
    } catch {
      /* no active pointer to capture (e.g. synthetic event) */
    }
    setIsDragging(true);
    soft?.classList.remove('cmp-rebounding');
    soft?.getAnimations().forEach(animation => animation.cancel());

    setTouchPoint(e.clientX, e.clientY, true);
    updateSoftness(1, 1, 0, 0);
    drag.frame = requestAnimationFrame(renderDrag);
  };

  const moveDrag = (e: React.PointerEvent<HTMLButtonElement>) => {
    const drag = dragRef.current;
    if (!drag.active || e.pointerId !== drag.pointerId) return;
    drag.pendingPoint = { clientX: e.clientX, clientY: e.clientY };
  };

  const endDrag = () => {
    const drag = dragRef.current;
    if (!drag.active) return;

    if (drag.frame) {
      cancelAnimationFrame(drag.frame);
      drag.frame = 0;
    }

    const wasPulled = drag.moved;
    drag.active = false;
    drag.pointerId = null;
    drag.pendingPoint = null;
    drag.rect = null;
    drag.skew = 0;
    drag.tilt = 0;

    setIsDragging(false);
    updateSoftness(1, 1, 0, 0);

    if (!reduceMotionRef.current) {
      const soft = softRef.current;
      soft?.classList.add('cmp-rebounding');
      window.setTimeout(() => soft?.classList.remove('cmp-rebounding'), 620);
    }

    // A real stretch-and-release reads as an affectionate squish: reward it with
    // a fresh in-character line and a little burst of hearts.
    if (wasPulled) requestMessage('pull');
  };
  endDragRef.current = endDrag;

  // A quick squash-and-spring without a drag — used for taps and keyboard boops.
  const boing = () => {
    if (reduceMotionRef.current) return;
    const soft = softRef.current;
    if (!soft) return;
    soft.classList.add('cmp-rebounding');
    updateSoftness(1.18, 0.88, -5, -4);
    window.setTimeout(() => {
      updateSoftness(1, 1, 0, 0);
      soft.classList.remove('cmp-rebounding');
    }, 520);
  };

  // Ask the companion for a new line. Guarded so rapid pokes don't stack
  // overlapping requests.
  const requestMessage = async (kind: 'poke' | 'pull') => {
    if (isLookingAtScreen || pendingMsgRef.current) return;
    pendingMsgRef.current = true;
    setIsTyping(true);
    addParticles(kind === 'pull' ? 3 : 4, kind === 'pull' ? 'heart' : 'star');
    try {
      const msg = await getCompanionMessage(stats, goals);
      if (mountedRef.current) setMessage(msg);
    } catch {
      /* keep the previous message */
    } finally {
      pendingMsgRef.current = false;
      if (mountedRef.current) setIsTyping(false);
    }
  };

  const handleKeyDown = (e: React.KeyboardEvent<HTMLButtonElement>) => {
    if (isLookingAtScreen) return;
    if (e.key === 'Escape') {
      endDrag();
      return;
    }
    if (e.key === ' ' || e.key === 'Enter') {
      e.preventDefault();
      boing();
      requestMessage('poke');
    }
  };

  // React when the user logs food: the calorie/protein totals jump, so Nutri
  // does a happy squish, hums a few notes, then delivers a progress message.
  useEffect(() => {
    if (stats.calories === 0) return;

    if (timeoutRef.current) clearTimeout(timeoutRef.current);

    setIsTyping(true);
    boing();
    const noteInterval = setInterval(() => {
      if (!isLookingAtScreen) addParticles(1, 'note');
    }, 400);

    timeoutRef.current = setTimeout(async () => {
      try {
        const msg = await getCompanionMessage(stats, goals);
        if (mountedRef.current) {
          setMessage(msg);
          addParticles(3, 'heart');
        }
      } catch {
        // keep old message
      } finally {
        if (mountedRef.current) setIsTyping(false);
        clearInterval(noteInterval);
      }
    }, 1500);

    return () => {
      if (timeoutRef.current) clearTimeout(timeoutRef.current);
      clearInterval(noteInterval);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [stats.calories, stats.protein]);

  // Safety net: finish any drag if the pointer is released off the element or
  // the window loses focus mid-grab.
  useEffect(() => {
    const finish = () => endDragRef.current();
    window.addEventListener('pointerup', finish);
    window.addEventListener('blur', finish);
    return () => {
      window.removeEventListener('pointerup', finish);
      window.removeEventListener('blur', finish);
    };
  }, []);

  return (
    <div className="cmp fixed bottom-24 right-4 md:bottom-8 md:left-24 md:right-auto z-40 flex flex-col items-center gap-2 pointer-events-none">
      <style>{CMP_STYLES}</style>

      {/* Speech Bubble */}
      <div
        className={`pointer-events-auto relative bg-white border border-[#e7e0ec] rounded-2xl p-3 shadow-lg max-w-[200px] mb-2 transform transition-all duration-300 origin-bottom ${
          message && !isLookingAtScreen ? 'scale-100 opacity-100' : 'scale-0 opacity-0'
        }`}
      >
        <div className="text-sm text-[#1d1b20] font-medium leading-tight">
          {isTyping ? (
            <div className="flex gap-1 items-center h-5">
              <span className="w-1.5 h-1.5 bg-[#6750a4] rounded-full animate-bounce" style={{ animationDelay: '0ms' }} />
              <span className="w-1.5 h-1.5 bg-[#6750a4] rounded-full animate-bounce" style={{ animationDelay: '150ms' }} />
              <span className="w-1.5 h-1.5 bg-[#6750a4] rounded-full animate-bounce" style={{ animationDelay: '300ms' }} />
            </div>
          ) : message}
        </div>
        {/* Tail */}
        <div className="absolute -bottom-2 left-1/2 -translate-x-1/2 w-4 h-4 bg-white border-b border-r border-[#e7e0ec] transform rotate-45" />
      </div>

      {/* Character — grab and stretch it */}
      <button
        ref={blobRef}
        type="button"
        aria-label="Nutri, your nutrition companion. Drag to stretch, press space to boop."
        className={`cmp-blob w-28 h-28 pointer-events-auto ${isLookingAtScreen ? 'is-away' : ''} ${isDragging ? 'is-dragging' : ''}`}
        onPointerDown={startDrag}
        onPointerMove={moveDrag}
        onPointerUp={endDrag}
        onPointerCancel={endDrag}
        onLostPointerCapture={endDrag}
        onKeyDown={handleKeyDown}
      >
        <span className="cmp-shadow" aria-hidden="true" />

        {/* Little round tail nub on the lower-back — a soft light bump that
            shows in the turned-away pose, like a bobtail seen from behind. */}
        <span className="cmp-tail" aria-hidden="true" />

        <span className="cmp-sprout" aria-hidden="true">
          <svg viewBox="95 -75 50 88" focusable="false">
            <path className="cmp-sprout-leaf" d="M120,24 Q120,0 92,5 Q120,12 120,24" />
            <path className="cmp-sprout-leaf" d="M120,24 Q120,-7 137,0 Q120,18 120,24" />
          </svg>
        </span>

        <span className="cmp-soft" aria-hidden="true" ref={softRef}>
          <span className="cmp-gloss" />
          <span className="cmp-touch" />
        </span>

        <svg className="cmp-face" viewBox="0 0 100 72" aria-hidden="true">
          <defs>
            <filter id="cmp-cheekBlur" x="-60%" y="-60%" width="220%" height="220%">
              <feGaussianBlur stdDeviation="1.7" />
            </filter>
          </defs>

          {/* Cheeks (shown in the forward-facing states) */}
          <g className="cmp-face-cheeks" filter="url(#cmp-cheekBlur)">
            <circle className="cmp-cheek" cx="12" cy="48" r="17" />
            <circle className="cmp-cheek" cx="86" cy="48" r="17" />
          </g>

          {/* Resting expression: round eyes + smile */}
          <g className="cmp-face-rest">
            <circle className="cmp-eye" cx="33" cy="27" r="5.5" />
            <circle className="cmp-eye" cx="67" cy="27" r="5.5" />
            <circle className="cmp-eye-shine" cx="30.4" cy="24.4" r="2.3" />
            <circle className="cmp-eye-shine" cx="64.4" cy="24.4" r="2.3" />
            <path className="cmp-smile" d="M37 48 Q50 60 63 48" />
          </g>

          {/* Pulling expression: squeezed >< eyes + open mouth */}
          <g className="cmp-face-pull">
            <path className="cmp-squint" d="M23 19 L37 28 L23 37" />
            <path className="cmp-squint" d="M77 19 L63 28 L77 37" />
            <ellipse className="cmp-ooh" cx="50" cy="50" rx="6.5" ry="8.5" />
          </g>

          {/* Turned-away pose: a single side-eye while the user reads content.
              Cheek + eye sit toward the right (front) edge of the profile, and
              the eye is a sharp chevron so it reads clearly as an eye. */}
          <g className="cmp-face-away">
            <circle className="cmp-cheek" cx="98" cy="46" r="13" filter="url(#cmp-cheekBlur)" />
            <path className="cmp-sideeye" d="M96 24 L107 31 L96 38" />
          </g>
        </svg>

        {/* Floating delight particles */}
        <span className="cmp-particles" aria-hidden="true">
          {particles.map((p) => (
            <span
              key={p.id}
              className="cmp-particle"
              style={{ left: `${p.x}%`, top: `${p.y}%`, transform: `scale(${p.scale})` }}
            >
              {p.type === 'heart' && (
                <svg width="20" height="20" viewBox="0 0 24 24" fill="#FFB7B2" className="drop-shadow-sm">
                  <path d="M12 21.35l-1.45-1.32C5.4 15.36 2 12.28 2 8.5 2 5.42 4.42 3 7.5 3c1.74 0 3.41.81 4.5 2.09C13.09 3.81 14.76 3 16.5 3 19.58 3 22 5.42 22 8.5c0 3.78-3.4 6.86-8.55 11.54L12 21.35z" />
                </svg>
              )}
              {p.type === 'star' && (
                <svg width="24" height="24" viewBox="0 0 24 24" fill="#FFD700" className="drop-shadow-sm">
                  <path d="M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z" />
                </svg>
              )}
              {p.type === 'note' && (
                <svg width="16" height="16" viewBox="0 0 24 24" fill="#6750a4" className="drop-shadow-sm">
                  <path d="M12 3v10.55c-.59-.34-1.27-.55-2-.55-2.21 0-4 1.79-4 4s1.79 4 4 4 4-1.79 4-4V7h4V3h-6z" />
                </svg>
              )}
            </span>
          ))}
        </span>
      </button>
    </div>
  );
};

// Scoped styles for the soft-object character. Every selector is namespaced
// under `cmp-` so nothing leaks into the rest of the app. Adapted from the
// standalone prototype's stylesheet (sized down for a corner widget and
// re-themed off the green prototype background onto the app's light surface).
const CMP_STYLES = `
.cmp-blob {
  --tilt: 0deg;
  position: relative;
  display: grid;
  place-items: center;
  padding: 0;
  border: 0;
  background: transparent;
  cursor: grab;
  touch-action: none;
  transform: rotate(var(--tilt));
  transition: transform 180ms cubic-bezier(0.2, 0.8, 0.2, 1);
  will-change: transform;
}
.cmp-blob:focus-visible {
  outline: 3px solid rgb(103 80 164 / 70%);
  outline-offset: 8px;
  border-radius: 42%;
}
.cmp-blob.is-dragging { cursor: grabbing; transition: none; }
.cmp-blob.is-away { transform: translateX(4px) rotate(8deg); cursor: default; }

.cmp-shadow {
  width: 72%;
  height: 15%;
  position: absolute;
  bottom: 2%;
  left: 50%;
  z-index: 0;
  border-radius: 50%;
  background: rgb(20 20 20 / 16%);
  filter: blur(8px);
  transform: translateX(-50%);
}

.cmp-tail {
  width: 11%;
  height: 11%;
  position: absolute;
  left: 27%;
  bottom: 27%;
  z-index: 2;
  border-radius: 50%;
  background: radial-gradient(circle at 42% 38%, #f5f4f9 0%, #e7e4ef 58%, rgba(231, 228, 239, 0) 100%);
  opacity: 0;
  transition: opacity 220ms ease;
}
.cmp-blob.is-away .cmp-tail { opacity: 1; }

.cmp-soft {
  --scale-x: 1;
  --scale-y: 1;
  --skew: 0deg;
  --touch-x: 50%;
  --touch-y: 50%;
  --origin-x: 50%;
  --origin-y: 50%;
  width: 78%;
  height: 70%;
  position: relative;
  display: block;
  overflow: hidden;
  z-index: 1;
  border: 2px solid rgb(64 74 84 / 18%);
  border-radius: 58% 42% 53% 47% / 50% 58% 42% 50%;
  background:
    radial-gradient(circle at var(--touch-x) var(--touch-y), rgb(255 255 255 / 78%) 0 8%, transparent 26%),
    radial-gradient(circle at 30% 22%, rgb(255 255 255 / 74%), transparent 26%),
    radial-gradient(circle at 72% 78%, rgb(170 182 196 / 22%), transparent 30%),
    linear-gradient(145deg, #f7f9fb, #ffffff 46%, #e9edf2);
  box-shadow:
    inset 6px 7px 11px rgb(255 255 255 / 46%),
    inset -8px -9px 14px rgb(128 145 164 / 22%),
    0 8px 18px rgb(78 86 92 / 18%);
  transform: scale(var(--scale-x), var(--scale-y)) skew(var(--skew));
  transform-origin: var(--origin-x) var(--origin-y);
  transition:
    transform 280ms cubic-bezier(0.2, 0.9, 0.2, 1.25),
    border-radius 280ms ease,
    box-shadow 280ms ease;
  will-change: transform;
  backface-visibility: hidden;
}
.cmp-blob.is-dragging .cmp-soft {
  border-radius: 62% 38% 56% 44% / 44% 61% 39% 56%;
  box-shadow:
    inset 7px 8px 12px rgb(255 255 255 / 40%),
    inset -10px -10px 16px rgb(128 145 164 / 26%),
    0 12px 22px rgb(78 86 92 / 20%);
  transition: border-radius 120ms ease, box-shadow 120ms ease;
}

.cmp-gloss {
  width: 34%;
  height: 22%;
  position: absolute;
  top: 18%;
  left: 19%;
  border-radius: 50%;
  background: rgb(255 255 255 / 48%);
  filter: blur(1px);
  transform: rotate(-18deg);
}

.cmp-touch {
  width: 30px;
  height: 30px;
  position: absolute;
  top: var(--touch-y);
  left: var(--touch-x);
  border: 2px solid rgb(255 255 255 / 50%);
  border-radius: 50%;
  opacity: 0;
  transform: translate(-50%, -50%) scale(0.5);
  transition: opacity 160ms ease, transform 160ms ease;
}
.cmp-blob.is-dragging .cmp-touch { opacity: 1; transform: translate(-50%, -50%) scale(1); }

.cmp-face {
  position: absolute;
  left: 50%;
  top: 50%;
  z-index: 3;
  display: block;
  width: 52%;
  height: auto;
  overflow: visible;
  pointer-events: none;
  transform: translate(-50%, -52%);
}
.cmp-face-cheeks,
.cmp-face-rest,
.cmp-face-pull,
.cmp-face-away { transition: opacity 220ms ease; }
.cmp-face-pull,
.cmp-face-away { opacity: 0; }
.cmp-blob.is-dragging .cmp-face-rest { opacity: 0; }
.cmp-blob.is-dragging .cmp-face-pull { opacity: 1; }
.cmp-blob.is-away .cmp-face-rest,
.cmp-blob.is-away .cmp-face-pull,
.cmp-blob.is-away .cmp-face-cheeks { opacity: 0; }
.cmp-blob.is-away .cmp-face-away { opacity: 1; }

.cmp-eye { fill: #1d1b20; }
.cmp-eye-shine { fill: rgb(255 255 255 / 92%); }
.cmp-smile { fill: none; stroke: #1d1b20; stroke-width: 3; stroke-linecap: round; }
.cmp-squint { fill: none; stroke: #1d1b20; stroke-width: 3.5; stroke-linecap: round; stroke-linejoin: round; }
.cmp-ooh { fill: #1d1b20; }
.cmp-sideeye { fill: none; stroke: #1d1b20; stroke-width: 3; stroke-linecap: round; stroke-linejoin: round; }
.cmp-cheek {
  fill: rgb(255 183 178);
  transform-box: fill-box;
  transform-origin: center;
  transition: transform 240ms cubic-bezier(0.2, 0.8, 0.2, 1.2);
}
.cmp-blob.is-dragging .cmp-cheek { transform: scale(1.14); }

.cmp-sprout {
  width: 24px;
  position: absolute;
  left: 50%;
  bottom: 78%;
  z-index: 2;
  pointer-events: none;
  transform: translateX(-50%) rotate(0deg);
  transform-origin: 50% 100%;
  animation: cmp-sprout-sway 4.2s ease-in-out infinite;
}
.cmp-sprout svg { display: block; width: 100%; height: auto; overflow: visible; filter: drop-shadow(0 3px 4px rgb(62 72 20 / 18%)); }
.cmp-sprout-leaf { fill: #74b843; stroke: #4d7f24; stroke-width: 3; stroke-linejoin: round; }

.cmp-soft.cmp-rebounding { animation: cmp-rebound 620ms cubic-bezier(0.2, 0.8, 0.16, 1.25); }

.cmp-particles { position: absolute; inset: 0; pointer-events: none; overflow: visible; z-index: 4; }
.cmp-particle { position: absolute; animation: cmp-float-up 1.5s ease-out forwards; }

@keyframes cmp-rebound {
  0% { transform: scale(1.2, 0.84) skew(-7deg); }
  34% { transform: scale(0.88, 1.12) skew(4deg); }
  62% { transform: scale(1.06, 0.95) skew(-2deg); }
  100% { transform: scale(1, 1) skew(0deg); }
}
@keyframes cmp-sprout-sway {
  0%, 100% { transform: translateX(-50%) rotate(-3deg); }
  50% { transform: translateX(-50%) rotate(4deg); }
}
@keyframes cmp-float-up {
  0% { transform: translateY(0) scale(0.5); opacity: 0; }
  20% { opacity: 1; transform: translateY(-8px) scale(1); }
  100% { transform: translateY(-40px) scale(0.8); opacity: 0; }
}
@media (prefers-reduced-motion: reduce) {
  .cmp-sprout { animation-duration: 1ms; }
  .cmp-soft { transition-duration: 1ms; }
}
`;

export default CompanionCharacter;
