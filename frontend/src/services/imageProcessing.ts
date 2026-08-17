const MAX_SOURCE_BYTES = 12 * 1024 * 1024;
const MAX_OUTPUT_BYTES = 6 * 1024 * 1024;
const MAX_EDGE = 1_600;
const JPEG_QUALITY = 0.82;

const SUPPORTED_SOURCE_TYPES = new Set([
  "image/jpeg",
  "image/png",
  "image/webp",
  "image/heic",
  "image/heif",
]);

export class PhotoValidationError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "PhotoValidationError";
  }
}

export function validateFoodPhoto(file: File): void {
  if (!SUPPORTED_SOURCE_TYPES.has(file.type.toLowerCase())) {
    throw new PhotoValidationError("Choose a JPG, PNG, WebP, HEIC, or HEIF photo.");
  }
  if (file.size <= 0) {
    throw new PhotoValidationError("That photo appears to be empty. Choose another one.");
  }
  if (file.size > MAX_SOURCE_BYTES) {
    throw new PhotoValidationError("That photo is over 12 MB. Choose a smaller image or lower your camera resolution.");
  }
}

async function decodePhoto(file: File): Promise<ImageBitmap> {
  try {
    return await createImageBitmap(file, { imageOrientation: "from-image" });
  } catch {
    throw new PhotoValidationError(
      "This photo could not be opened. Try a JPG from your camera or choose another image."
    );
  }
}

function canvasToJpeg(canvas: HTMLCanvasElement, quality: number): Promise<Blob> {
  return new Promise((resolve, reject) => {
    canvas.toBlob(
      (blob) => {
        if (blob) resolve(blob);
        else reject(new PhotoValidationError("The photo could not be prepared. Try another image."));
      },
      "image/jpeg",
      quality
    );
  });
}

/**
 * Normalizes camera/gallery input before upload. Drawing to canvas applies the
 * captured orientation, removes EXIF metadata, and keeps the base64 request
 * comfortably below the backend limit on typical Android devices.
 */
export async function prepareFoodPhoto(file: File): Promise<File> {
  validateFoodPhoto(file);
  const bitmap = await decodePhoto(file);

  try {
    const scale = Math.min(1, MAX_EDGE / Math.max(bitmap.width, bitmap.height));
    const width = Math.max(1, Math.round(bitmap.width * scale));
    const height = Math.max(1, Math.round(bitmap.height * scale));
    const canvas = document.createElement("canvas");
    canvas.width = width;
    canvas.height = height;

    const context = canvas.getContext("2d", { alpha: false });
    if (!context) throw new PhotoValidationError("The photo could not be prepared on this device.");
    context.fillStyle = "#ffffff";
    context.fillRect(0, 0, width, height);
    context.drawImage(bitmap, 0, 0, width, height);

    let blob = await canvasToJpeg(canvas, JPEG_QUALITY);
    if (blob.size > MAX_OUTPUT_BYTES) blob = await canvasToJpeg(canvas, 0.68);
    if (blob.size > MAX_OUTPUT_BYTES) {
      throw new PhotoValidationError("The prepared photo is still too large. Try a tighter crop.");
    }

    const stem = file.name.replace(/\.[^.]+$/, "") || "meal";
    return new File([blob], `${stem}.jpg`, {
      type: "image/jpeg",
      lastModified: Date.now(),
    });
  } finally {
    bitmap.close();
  }
}

