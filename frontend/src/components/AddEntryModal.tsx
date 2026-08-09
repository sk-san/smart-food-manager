import React, { useCallback, useEffect, useRef, useState } from 'react';
import { X, Camera, Type, Loader2, Check, Upload } from 'lucide-react';
import { analyzeFoodInput } from '../services/nutritionService';
import { FoodEntry } from '../types/nutrition';

interface AddEntryModalProps {
  isOpen: boolean;
  onClose: () => void;
  onAdd: (items: Omit<FoodEntry, 'id' | 'timestamp'>[]) => void;
}

const TITLE_ID = 'log-food-title';
const TEXT_INPUT_ID = 'log-food-description';

// Everything the trap will cycle through. The file input is deliberately not
// reachable — it is `display: none` and driven by its own button — and the
// visibility filter below drops it along with anything else not laid out.
const FOCUSABLE_SELECTOR = [
  'a[href]',
  'button:not([disabled])',
  'textarea:not([disabled])',
  'input:not([disabled])',
  'select:not([disabled])',
  '[tabindex]:not([tabindex="-1"])',
].join(',');

const AddEntryModal: React.FC<AddEntryModalProps> = ({ isOpen, onClose, onAdd }) => {
  const [mode, setMode] = useState<'text' | 'image'>('text');
  const [inputText, setInputText] = useState('');
  const [selectedFile, setSelectedFile] = useState<File | null>(null);
  const [previewUrl, setPreviewUrl] = useState<string | null>(null);
  const [isAnalyzing, setIsAnalyzing] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [analysisResult, setAnalysisResult] = useState<Omit<FoodEntry, 'id' | 'timestamp'>[] | null>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const dialogRef = useRef<HTMLDivElement>(null);

  // Closing resets the dialog to its opening state, so reopening never shows
  // the last attempt's text, photo or error.
  const handleClose = useCallback(() => {
    setInputText('');
    setSelectedFile(null);
    setPreviewUrl(null);
    setAnalysisResult(null);
    setError(null);
    setMode('text');
    onClose();
  }, [onClose]);

  // Focus moves into the dialog on open and back to whatever opened it on
  // close — otherwise closing drops the caret at the top of the document and a
  // keyboard reader has to tab all the way back down.
  useEffect(() => {
    if (!isOpen) return;
    const opener = document.activeElement as HTMLElement | null;
    // Focus the container rather than a control, so the role and accessible
    // name are announced before the first field.
    dialogRef.current?.focus();
    return () => {
      // The mobile trigger unmounts while the dialog is up, so only refocus
      // something still in the document.
      if (opener?.isConnected) opener.focus();
    };
  }, [isOpen]);

  // Escape listens on the document, not on the dialog. Focus is inside the
  // dialog in the normal case, but it need not be — a click on the backdrop
  // leaves it on <body> — and Escape has to close the dialog either way.
  useEffect(() => {
    if (!isOpen) return;
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.stopPropagation();
        handleClose();
      }
    };
    document.addEventListener('keydown', onKeyDown);
    return () => document.removeEventListener('keydown', onKeyDown);
  }, [isOpen, handleClose]);

  // Hold the page still underneath. Without this, scrolling inside the dialog
  // on a phone scrolls the dashboard behind it once the panel hits its end.
  useEffect(() => {
    if (!isOpen) return;
    const previous = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
    return () => {
      document.body.style.overflow = previous;
    };
  }, [isOpen]);

  // Object URLs outlive the component unless they are handed back.
  useEffect(() => {
    if (!previewUrl) return;
    return () => URL.revokeObjectURL(previewUrl);
  }, [previewUrl]);

  if (!isOpen) return null;

  const handleFileSelect = (e: React.ChangeEvent<HTMLInputElement>) => {
    if (e.target.files && e.target.files[0]) {
      const file = e.target.files[0];
      setSelectedFile(file);
      setPreviewUrl(URL.createObjectURL(file));
      setAnalysisResult(null);
      setError(null);
    }
  };

  const handleAnalyze = async () => {
    if (mode === 'text' && !inputText.trim()) return;
    if (mode === 'image' && !selectedFile) return;

    setIsAnalyzing(true);
    setAnalysisResult(null);
    setError(null);

    try {
      const input = mode === 'text' ? inputText : selectedFile!;
      const result = await analyzeFoodInput(input, mode);
      if (result.length === 0) {
        setError('No food was recognised in that. Try describing it in a few more words.');
        return;
      }
      setAnalysisResult(result);
    } catch (err) {
      console.error(err);
      setError('That could not be analysed. Check your connection and try again.');
    } finally {
      setIsAnalyzing(false);
    }
  };

  const handleConfirm = () => {
    if (analysisResult) {
      onAdd(analysisResult);
      handleClose();
    }
  };

  // Tab is corralled so focus never escapes to the page behind. (Escape is
  // handled on the document, since focus is not always inside the dialog.)
  const handleKeyDown = (e: React.KeyboardEvent<HTMLDivElement>) => {
    if (e.key !== 'Tab' || !dialogRef.current) return;

    const focusable = Array.from(
      dialogRef.current.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTOR)
    ).filter((el) => el.offsetParent !== null);
    if (focusable.length === 0) return;

    const first = focusable[0];
    const last = focusable[focusable.length - 1];
    const active = document.activeElement;

    if (e.shiftKey && (active === first || active === dialogRef.current)) {
      e.preventDefault();
      last.focus();
    } else if (!e.shiftKey && active === last) {
      e.preventDefault();
      first.focus();
    }
  };

  const segOption = (target: 'text' | 'image', icon: React.ReactNode, label: string) => (
    <button
      type="button"
      aria-pressed={mode === target}
      onClick={() => {
        setMode(target);
        setAnalysisResult(null);
        setError(null);
      }}
      className={`flex items-center gap-1.5 px-4 py-2.5 text-[13px] transition-colors ${
        mode === target
          ? 'bg-accent-solid font-semibold text-bg'
          : 'text-neutral-700 hover:bg-neutral-200'
      }`}
    >
      {icon}
      <span>{label}</span>
    </button>
  );

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-neutral-900/50 p-4 backdrop-blur-sm transition-opacity dark:bg-black/60"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) handleClose();
      }}
    >
      <div
        ref={dialogRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby={TITLE_ID}
        tabIndex={-1}
        onKeyDown={handleKeyDown}
        className="flex max-h-[90vh] w-full max-w-lg flex-col overflow-hidden rounded-card bg-surface shadow-lg focus:outline-none"
      >
        {/* Header */}
        <div className="flex items-center justify-between p-6 pb-2">
          <h2 id={TITLE_ID} className="text-[24px] text-ink">
            Log Food
          </h2>
          <button
            type="button"
            onClick={handleClose}
            className="grid h-11 w-11 place-items-center rounded-full text-neutral-700 transition-colors hover:bg-neutral-200"
            aria-label="Close"
          >
            <X size={22} strokeWidth={2.5} />
          </button>
        </div>

        {/* Mode segment */}
        <div
          role="group"
          aria-label="How to describe the food"
          className="mx-6 mt-2 inline-flex self-start overflow-hidden rounded-full border border-divider"
        >
          {segOption('text', <Type size={16} strokeWidth={2.5} />, 'Text')}
          <div className="w-px bg-divider" />
          {segOption('image', <Camera size={16} strokeWidth={2.5} />, 'Image')}
        </div>

        {/* Content */}
        <div className="flex-1 overflow-y-auto p-6">
          {!analysisResult ? (
            <div className="flex flex-col gap-4">
              {mode === 'text' ? (
                <div>
                  <label className="field-label" htmlFor={TEXT_INPUT_ID}>
                    What did you eat?
                  </label>
                  <textarea
                    id={TEXT_INPUT_ID}
                    className="input h-40 resize-none rounded-3xl px-4 py-3"
                    placeholder="e.g., A grilled chicken sandwich and a medium fries..."
                    value={inputText}
                    onChange={(e) => setInputText(e.target.value)}
                  />
                </div>
              ) : (
                <div>
                  <span className="field-label" id="log-food-photo-label">
                    Food photo
                  </span>
                  {/* A real button, so the picker opens on Enter and Space and
                      the control appears in the tab order. */}
                  <button
                    type="button"
                    aria-labelledby="log-food-photo-label"
                    aria-describedby="log-food-photo-hint"
                    onClick={() => fileInputRef.current?.click()}
                    className="relative flex h-64 w-full cursor-pointer flex-col items-center justify-center overflow-hidden rounded-3xl border-2 border-dashed border-neutral-500 transition-colors hover:bg-neutral-100"
                  >
                    {previewUrl ? (
                      <img src={previewUrl} alt="" className="absolute inset-0 h-full w-full object-cover" />
                    ) : (
                      <>
                        <Upload size={30} strokeWidth={2.5} className="mb-2 text-neutral-600" />
                        <span id="log-food-photo-hint" className="text-sm text-neutral-700">
                          Choose a food photo
                        </span>
                      </>
                    )}
                  </button>
                  {selectedFile && (
                    <p className="mt-2 truncate text-[13px] text-neutral-700">
                      Selected: {selectedFile.name}
                    </p>
                  )}
                  <input
                    type="file"
                    ref={fileInputRef}
                    className="hidden"
                    accept="image/*"
                    tabIndex={-1}
                    aria-hidden="true"
                    onChange={handleFileSelect}
                  />
                </div>
              )}

              {/* Progress and failure both reach a screen reader without
                  stealing focus. */}
              <p role="status" aria-live="polite" className="sr-only">
                {isAnalyzing ? 'Analyzing your food…' : ''}
              </p>
              {error && (
                <p role="alert" className="rounded-2xl bg-accent-100 px-4 py-3 text-[13px] text-accent-800">
                  {error}
                </p>
              )}

              <button
                type="button"
                disabled={isAnalyzing || (mode === 'text' ? !inputText : !selectedFile)}
                onClick={handleAnalyze}
                className="btn btn-primary w-full py-3.5 text-[15px] shadow-sm"
              >
                {isAnalyzing ? (
                  <>
                    <Loader2 className="animate-spin" size={18} aria-hidden="true" />
                    Analyzing…
                  </>
                ) : (
                  <React.Fragment>
                    Analyze{' '}
                    <span className="rounded-full bg-accent-900 px-2 py-0.5 font-sans text-xs font-semibold">
                      AI
                    </span>
                  </React.Fragment>
                )}
              </button>
            </div>
          ) : (
            <div className="animate-in fade-in slide-in-from-bottom-4 flex flex-col gap-4 duration-500">
              <h3 className="text-[19px] text-ink">Found Items</h3>
              <p role="status" aria-live="polite" className="sr-only">
                {`Analysis finished: ${analysisResult.length} ${
                  analysisResult.length === 1 ? 'item' : 'items'
                } found. Review and add them to your log.`}
              </p>
              <ul className="flex list-none flex-col gap-2.5">
                {analysisResult.map((item, idx) => (
                  <li key={idx} className="flex items-center justify-between gap-3 rounded-3xl bg-bg p-4">
                    <div className="min-w-0">
                      <p className="truncate text-sm font-semibold text-ink">{item.name}</p>
                      <p className="text-xs text-neutral-700 tabular-nums">{item.calories} kcal</p>
                    </div>
                    <div className="flex shrink-0 gap-1.5">
                      <span className="tag tag-accent tabular-nums">P {item.protein}g</span>
                      <span className="tag tag-accent-2 tabular-nums">C {item.carbs}g</span>
                      <span className="tag tag-neutral tabular-nums">F {item.fat}g</span>
                    </div>
                  </li>
                ))}
              </ul>
              <div className="flex gap-2.5 pt-3">
                <button
                  type="button"
                  onClick={() => setAnalysisResult(null)}
                  className="btn btn-secondary flex-1 py-3"
                >
                  Retry
                </button>
                <button type="button" onClick={handleConfirm} className="btn btn-primary flex-1 py-3 shadow-sm">
                  <Check size={17} strokeWidth={2.75} />
                  Add to Log
                </button>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
};

export default AddEntryModal;
