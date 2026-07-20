import React, { useState, useRef } from 'react';
import { X, Camera, Type, Loader2, Check, Upload } from 'lucide-react';
import { analyzeFoodInput } from '../services/nutritionService';
import { FoodEntry } from '../types/nutrition';

interface AddEntryModalProps {
  isOpen: boolean;
  onClose: () => void;
  onAdd: (items: Omit<FoodEntry, 'id' | 'timestamp'>[]) => void;
}

const AddEntryModal: React.FC<AddEntryModalProps> = ({ isOpen, onClose, onAdd }) => {
  const [mode, setMode] = useState<'text' | 'image'>('text');
  const [inputText, setInputText] = useState('');
  const [selectedFile, setSelectedFile] = useState<File | null>(null);
  const [previewUrl, setPreviewUrl] = useState<string | null>(null);
  const [isAnalyzing, setIsAnalyzing] = useState(false);
  const [analysisResult, setAnalysisResult] = useState<Omit<FoodEntry, 'id' | 'timestamp'>[] | null>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);

  if (!isOpen) return null;

  const handleFileSelect = (e: React.ChangeEvent<HTMLInputElement>) => {
    if (e.target.files && e.target.files[0]) {
      const file = e.target.files[0];
      setSelectedFile(file);
      setPreviewUrl(URL.createObjectURL(file));
      setAnalysisResult(null);
    }
  };

  const handleAnalyze = async () => {
    if (mode === 'text' && !inputText.trim()) return;
    if (mode === 'image' && !selectedFile) return;

    setIsAnalyzing(true);
    setAnalysisResult(null);

    try {
      const input = mode === 'text' ? inputText : selectedFile!;
      const result = await analyzeFoodInput(input, mode);
      setAnalysisResult(result);
    } catch (err) {
      console.error(err);
      alert('Failed to analyze. Please try again.');
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

  const handleClose = () => {
    setInputText('');
    setSelectedFile(null);
    setPreviewUrl(null);
    setAnalysisResult(null);
    setMode('text');
    onClose();
  };

  const segOption = (target: 'text' | 'image', icon: React.ReactNode, label: string) => (
    <button
      onClick={() => { setMode(target); setAnalysisResult(null); }}
      className={`flex items-center gap-1.5 px-4 py-2 text-[13px] transition-colors ${
        mode === target
          ? 'bg-accent font-semibold text-bg'
          : 'text-neutral-700 hover:bg-neutral-200'
      }`}
    >
      {icon}
      <span>{label}</span>
    </button>
  );

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-neutral-900/50 p-4 backdrop-blur-sm transition-opacity dark:bg-black/60">
      <div className="flex max-h-[90vh] w-full max-w-lg flex-col overflow-hidden rounded-card bg-surface shadow-lg">
        {/* Header */}
        <div className="flex items-center justify-between p-6 pb-2">
          <h2 className="text-[24px] text-ink">Log Food</h2>
          <button
            onClick={handleClose}
            className="rounded-full p-2 text-neutral-600 transition-colors hover:bg-neutral-200"
            aria-label="Close"
          >
            <X size={22} strokeWidth={2.5} />
          </button>
        </div>

        {/* Mode segment */}
        <div className="mx-6 mt-2 inline-flex self-start overflow-hidden rounded-full border border-divider">
          {segOption('text', <Type size={16} strokeWidth={2.5} />, 'Text')}
          <div className="w-px bg-divider" />
          {segOption('image', <Camera size={16} strokeWidth={2.5} />, 'Image')}
        </div>

        {/* Content */}
        <div className="flex-1 overflow-y-auto p-6">
          {!analysisResult ? (
            <div className="flex flex-col gap-4">
              {mode === 'text' ? (
                <textarea
                  className="input h-40 resize-none rounded-3xl px-4 py-3"
                  placeholder="e.g., A grilled chicken sandwich and a medium fries..."
                  value={inputText}
                  onChange={(e) => setInputText(e.target.value)}
                />
              ) : (
                <div
                  className="relative flex h-64 w-full cursor-pointer flex-col items-center justify-center overflow-hidden rounded-3xl border-2 border-dashed border-neutral-400 transition-colors hover:bg-neutral-100"
                  onClick={() => fileInputRef.current?.click()}
                >
                  {previewUrl ? (
                    <img src={previewUrl} alt="Preview" className="absolute inset-0 h-full w-full object-cover" />
                  ) : (
                    <>
                      <Upload size={30} strokeWidth={2.5} className="mb-2 text-neutral-500" />
                      <p className="text-sm text-neutral-600">Tap to upload food photo</p>
                    </>
                  )}
                  <input
                    type="file"
                    ref={fileInputRef}
                    className="hidden"
                    accept="image/*"
                    onChange={handleFileSelect}
                  />
                </div>
              )}

              <button
                disabled={isAnalyzing || (mode === 'text' ? !inputText : !selectedFile)}
                onClick={handleAnalyze}
                className="btn btn-primary w-full py-3.5 text-[15px] shadow-sm"
              >
                {isAnalyzing ? (
                  <Loader2 className="animate-spin" />
                ) : (
                  <React.Fragment>
                    Analyze{' '}
                    <span className="rounded-full bg-accent-700 px-2 py-0.5 font-sans text-[11px] font-semibold">
                      AI
                    </span>
                  </React.Fragment>
                )}
              </button>
            </div>
          ) : (
            <div className="animate-in fade-in slide-in-from-bottom-4 flex flex-col gap-4 duration-500">
              <h3 className="text-[19px] text-ink">Found Items</h3>
              <div className="flex flex-col gap-2.5">
                {analysisResult.map((item, idx) => (
                  <div key={idx} className="flex items-center justify-between gap-3 rounded-3xl bg-bg p-4">
                    <div className="min-w-0">
                      <p className="truncate text-sm font-semibold text-ink">{item.name}</p>
                      <p className="text-xs text-neutral-600 tabular-nums">{item.calories} kcal</p>
                    </div>
                    <div className="flex shrink-0 gap-1.5">
                      <span className="tag tag-accent tabular-nums">P {item.protein}g</span>
                      <span className="tag tag-accent-2 tabular-nums">C {item.carbs}g</span>
                      <span className="tag tag-neutral tabular-nums">F {item.fat}g</span>
                    </div>
                  </div>
                ))}
              </div>
              <div className="flex gap-2.5 pt-3">
                <button onClick={() => setAnalysisResult(null)} className="btn btn-secondary flex-1 py-3">
                  Retry
                </button>
                <button onClick={handleConfirm} className="btn btn-primary flex-1 py-3 shadow-sm">
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
