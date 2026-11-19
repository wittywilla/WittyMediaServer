# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 WittyWilla and contributors
"""Demucs (htdemucs) through ONNX Runtime, for graphics cards PyTorch can't use here
(Intel and AMD cards on Windows, through DirectML).

The server writes this file into its separator folder when it installs Demucs, and runs it
with that folder's Python. Nothing here downloads anything: the model's weights are the ones
the install fetched from Meta's own server, and they're converted here, on this machine.

    python demucs_onnx.py export   --out htdemucs.onnx [--check clip.wav]
    python demucs_onnx.py probe    --onnx htdemucs.onnx --provider dml
    python demucs_onnx.py separate --onnx htdemucs.onnx --provider dml -o OUT SONG

The spectrogram steps (STFT and its inverse) stay outside the ONNX graph and run with
PyTorch on the processor: ONNX has no complex numbers, and DirectML lacks what PyTorch's
STFT turns into. Everything between them, the network, runs on the card. Each part of a
song has the length the model was trained on, so the graph's shapes are fixed.
"""
import argparse
import math
import os
import sys
import time

import numpy as np
import torch
import torch.nn.functional as F

MODEL = "htdemucs"
OPSET = 17


def load_htdemucs():
    from demucs.pretrained import get_model
    from demucs.apply import BagOfModels

    bag = get_model(MODEL)
    bag.eval()
    inner = bag.models[0] if isinstance(bag, BagOfModels) else bag
    return bag, inner


def glu_sliced(x, dim=-1):
    n = x.shape[dim] // 2
    return x.narrow(dim, 0, n) * torch.sigmoid(x.narrow(dim, n, n))


def training_length(m):
    return int(m.segment * m.samplerate)


class Core(torch.nn.Module):
    """HTDemucs's forward without its spectrogram steps: takes the song's part and its
    spectrogram (as real numbers), gives the separated spectrograms and waveforms, each to be
    finished outside (the inverse STFT of the first, added to the second)."""

    def __init__(self, m):
        super().__init__()
        self.m = m

    def forward(self, mix, mag):
        m = self.m
        x = mag
        B, C, Fq, T = x.shape
        mean = x.mean(dim=(1, 2, 3), keepdim=True)
        std = x.std(dim=(1, 2, 3), keepdim=True)
        x = (x - mean) / (1e-5 + std)
        xt = mix
        meant = xt.mean(dim=(1, 2), keepdim=True)
        stdt = xt.std(dim=(1, 2), keepdim=True)
        xt = (xt - meant) / (1e-5 + stdt)
        saved, saved_t, lengths, lengths_t = [], [], [], []
        for idx, encode in enumerate(m.encoder):
            lengths.append(x.shape[-1])
            inject = None
            if idx < len(m.tencoder):
                lengths_t.append(xt.shape[-1])
                tenc = m.tencoder[idx]
                xt = tenc(xt)
                if not tenc.empty:
                    saved_t.append(xt)
                else:
                    inject = xt
            x = encode(x, inject)
            if idx == 0 and m.freq_emb is not None:
                frs = torch.arange(x.shape[-2], device=x.device)
                emb = m.freq_emb(frs).t()[None, :, :, None].expand_as(x)
                x = x + m.freq_emb_scale * emb
            saved.append(x)
        if m.crosstransformer:
            if m.bottom_channels:
                b, c, f, t = x.shape
                x = x.reshape(b, c, f * t)
                x = m.channel_upsampler(x)
                x = x.reshape(b, -1, f, t)
                xt = m.channel_upsampler_t(xt)
            x, xt = m.crosstransformer(x, xt)
            if m.bottom_channels:
                b, c, f, t = x.shape
                x = x.reshape(b, c, f * t)
                x = m.channel_downsampler(x)
                x = x.reshape(b, -1, f, t)
                xt = m.channel_downsampler_t(xt)
        for idx, decode in enumerate(m.decoder):
            skip = saved.pop(-1)
            x, pre = decode(x, skip, lengths.pop(-1))
            offset = m.depth - len(m.tdecoder)
            if idx >= offset:
                tdec = m.tdecoder[idx - offset]
                length_t = lengths_t.pop(-1)
                if tdec.empty:
                    pre = pre[:, :, 0]
                    xt, _ = tdec(pre, None, length_t)
                else:
                    skip = saved_t.pop(-1)
                    xt, _ = tdec(xt, skip, length_t)
        S = len(m.sources)
        x = x.view(B, S, -1, Fq, T)
        x = x * std[:, None] + mean[:, None]
        xt = xt.view(B, S, -1, mix.shape[-1])
        xt = xt * stdt[:, None] + meant[:, None]
        return x, xt


def session(path, provider):
    import onnxruntime as ort

    so = ort.SessionOptions()
    so.log_severity_level = 3
    if provider == "dml":
        # DirectML wants these off; see ONNX Runtime's DirectML notes.
        so.enable_mem_pattern = False
        so.execution_mode = ort.ExecutionMode.ORT_SEQUENTIAL
        providers = ["DmlExecutionProvider"]
    else:
        so.intra_op_num_threads = max(1, (os.cpu_count() or 2) - 1)
        providers = ["CPUExecutionProvider"]
    s = ort.InferenceSession(path, so, providers=providers)
    if provider == "dml" and "DmlExecutionProvider" not in s.get_providers():
        raise RuntimeError("DirectML isn't available here")
    return s


def onnx_forward(m, sess):
    """A forward for the model that runs its core in ONNX Runtime."""
    tl = training_length(m)

    def forward(mix):
        length = mix.shape[-1]
        pre = None
        if length < tl:
            pre = length
            mix = F.pad(mix, (0, tl - length))
        z = m._spec(mix)
        mag = m._magnitude(z)
        xs, xts = [], []
        for b in range(mix.shape[0]):
            x, xt = sess.run(None, {"mix": mix[b:b + 1].float().cpu().numpy(), "mag": mag[b:b + 1].float().cpu().numpy()})
            xs.append(torch.from_numpy(x))
            xts.append(torch.from_numpy(xt))
        x = torch.cat(xs)
        xt = torch.cat(xts)
        zout = m._mask(z, x)
        x = m._ispec(zout, tl)
        out = xt + x
        if pre:
            out = out[..., :pre]
        return out

    return forward


def snr(ref, est):
    ref = ref.double()
    err = (ref - est.double()).pow(2).sum().item()
    return 10 * math.log10(ref.pow(2).sum().item() / max(err, 1e-20))


def check_clip(path, m):
    """The first part of a song as the model takes it, or a made-up one."""
    tl = training_length(m)
    if path:
        from demucs.separate import load_track

        wav = load_track(path, m.audio_channels, m.samplerate)
        start = min(max(0, wav.shape[-1] // 3), max(0, wav.shape[-1] - tl))
        wav = wav[..., start:start + tl]
        wav = F.pad(wav, (0, tl - wav.shape[-1]))
    else:
        g = torch.Generator().manual_seed(7)
        t = torch.arange(tl) / m.samplerate
        tone = 0.3 * torch.sin(2 * math.pi * 220 * t) + 0.2 * torch.sin(2 * math.pi * 440 * t)
        wav = torch.stack([tone, tone * 0.8]) + 0.05 * torch.randn(2, tl, generator=g)
    ref = wav.mean(0)
    wav = (wav - ref.mean()) / ref.std().clamp_min(1e-8)
    return wav[None].float()


def cmd_export(a):
    torch.set_num_threads(max(1, (os.cpu_count() or 2) - 1))
    # Attention's fused fast path has no ONNX form; the ordinary path computes the same.
    torch.backends.mha.set_fastpath_enabled(False)
    # GLU exports as a Split feeding Sigmoid and Mul, which DirectML computes wrongly (checked
    # on ONNX Runtime 1.24.4); two slices compute the same and come out right everywhere.
    F.glu = glu_sliced
    _, m = load_htdemucs()
    tl = training_length(m)
    mix = check_clip(a.check, m)
    with torch.no_grad():
        z = m._spec(mix)
        mag = m._magnitude(z)
        core = Core(m).eval()
        tmp = a.out + ".part"
        t0 = time.time()
        torch.onnx.export(core, (mix, mag), tmp, input_names=["mix", "mag"], output_names=["x", "xt"],
                          opset_version=OPSET, do_constant_folding=True, dynamo=False)
        print(f"exported in {time.time() - t0:.0f} s", flush=True)
        # The same part through PyTorch, whole, on the processor: the export must agree with it.
        ref = m(mix)
    sess = session(tmp, "cpu")
    out = onnx_forward(m, sess)(mix)
    got = snr(ref, out)
    diff = (ref - out).abs().max().item()
    print(f"check: SNR {got:.1f} dB, largest difference {diff:.2e}", flush=True)
    if got < a.min_snr:
        os.remove(tmp)
        print(f"the converted model doesn't match Demucs (SNR {got:.1f} dB, needs {a.min_snr:.0f})", file=sys.stderr)
        return 3
    os.replace(tmp, a.out)
    print(f"samples {tl}, written {os.path.getsize(a.out) >> 20} MB", flush=True)
    return 0


def cmd_probe(a):
    """A tiny run on the card: does it work here, and agree with the processor?"""
    _, m = load_htdemucs()
    mix = check_clip(None, m)
    t0 = time.time()
    sess = session(a.onnx, a.provider)
    with torch.no_grad():
        out = onnx_forward(m, sess)(mix)
    took = time.time() - t0
    cpu = onnx_forward(m, session(a.onnx, "cpu"))(mix) if a.provider != "cpu" else out
    got = snr(cpu, out)
    print(f"{a.provider}: {took:.1f} s, SNR against the processor {got:.1f} dB", flush=True)
    if not torch.isfinite(out).all() or got < a.min_snr:
        print("the graphics card's answer doesn't match the processor's", file=sys.stderr)
        return 3
    return 0


def cmd_separate(a):
    from pathlib import Path
    from demucs.apply import apply_model
    from demucs.audio import save_audio
    from demucs.separate import load_track

    torch.set_num_threads(max(1, (os.cpu_count() or 2) - 1))
    bag, m = load_htdemucs()
    m.forward = onnx_forward(m, session(a.onnx, a.provider))
    for track in a.tracks:
        track = Path(track)
        wav = load_track(track, m.audio_channels, m.samplerate)
        ref = wav.mean(0)
        wav = wav - ref.mean()
        wav = wav / ref.std().clamp_min(1e-8)
        t0 = time.time()
        with torch.no_grad():
            sources = apply_model(bag, wav[None], device="cpu", shifts=1, split=True, overlap=0.25, progress=True)[0]
        sources = sources * ref.std() + ref.mean()
        out = Path(a.out) / MODEL / track.name.rsplit(".", 1)[0]
        out.mkdir(parents=True, exist_ok=True)
        for source, name in zip(sources, bag.sources):
            save_audio(source, str(out / f"{name}.wav"), samplerate=m.samplerate, clip="rescale", as_float=False, bits_per_sample=16)
        print(f"separated {track.name} in {time.time() - t0:.1f} s on {a.provider}", flush=True)
    return 0


def main():
    p = argparse.ArgumentParser(prog="demucs_onnx")
    sub = p.add_subparsers(dest="cmd", required=True)
    e = sub.add_parser("export")
    e.add_argument("--out", required=True)
    e.add_argument("--check", help="a song to check the conversion with (a made-up sound when left out)")
    e.add_argument("--min-snr", type=float, default=60.0)
    pr = sub.add_parser("probe")
    pr.add_argument("--onnx", required=True)
    pr.add_argument("--provider", choices=["dml", "cpu"], default="dml")
    pr.add_argument("--min-snr", type=float, default=30.0)
    s = sub.add_parser("separate")
    s.add_argument("--onnx", required=True)
    s.add_argument("--provider", choices=["dml", "cpu"], default="dml")
    s.add_argument("-o", "--out", required=True)
    s.add_argument("tracks", nargs="+")
    a = p.parse_args()
    return {"export": cmd_export, "probe": cmd_probe, "separate": cmd_separate}[a.cmd](a)


if __name__ == "__main__":
    sys.exit(main())
