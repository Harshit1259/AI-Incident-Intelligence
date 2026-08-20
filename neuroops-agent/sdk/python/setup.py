"""
NeurOps Agent — Python SDK
Copyright (c) NeurOps 2025. All rights reserved.
"""

from setuptools import setup, find_packages

setup(
    name="neuroopssdk",
    version="1.0.0",
    description="Official Python SDK for writing NeurOps Agent plugins",
    author="NeurOps Engineering",
    author_email="engg@neuroops.ai",
    packages=find_packages(),
    python_requires=">=3.9",
    install_requires=[
        "pyzmq>=22.3.0",
        "psutil>=5.8.0",
        "paramiko>=2.9.0",
        "pysnmp>=4.4.12",
        "requests>=2.27.0",
        "requests-ntlm>=1.1.0",
    ],
    extras_require={
        "vmware":   ["pyvmomi>=7.0.3"],
        "winrm":    ["pywinrm>=0.4.2"],
        "oracle":   ["cx-Oracle>=8.3.0"],
        "mongodb":  ["pymongo>=4.0.0"],
        "aws":      ["boto3>=1.20.0"],
    },
    classifiers=[
        "Programming Language :: Python :: 3",
        "Programming Language :: Python :: 3.9",
        "Operating System :: POSIX :: Linux",
        "Topic :: System :: Monitoring",
        "Topic :: System :: Systems Administration",
    ],
)
